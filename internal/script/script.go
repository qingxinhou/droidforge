// Package script 解释 BAT 语法的一个子集，把其中的 adb 操作转换成真实调用。
//
// 支持：@echo、echo、rem/::、set、set /a、变量替换、goto / 标签、
// if errorlevel / if exist / if "a"=="b"（含 not 与块形式）、for /l、for %%i in (..)、
// timeout /t、choice /t、pause、exit /b、adb 子命令。
//
// 不支持：call、start、powershell、for /f、管道与重定向、外部 EXE。
// 遇到不支持的命令会记录错误并继续，不会中断整个脚本。
package script

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Host 是脚本执行所需的外部能力。
type Host struct {
	Shell func(ctx context.Context, command string) (string, error)
	Adb   func(ctx context.Context, args []string) (string, error)
	Log   func(kind, text string)
}

// Result 是执行统计。
type Result struct {
	Steps    int `json:"steps"`
	Failures int `json:"failures"`
}

const (
	maxSteps    = 20000
	maxLoopIter = 2000
	maxSleepSec = 30
)

var (
	varPattern    = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)
	doublePattern = regexp.MustCompile(`%%([A-Za-z_][A-Za-z0-9_]*)`)
)

type state struct {
	host     Host
	vars     map[string]string
	labels   map[string]int
	echo     bool
	errlevel int
	steps    int
	failures int
	lastOut  string
}

// Run 执行脚本。
func Run(ctx context.Context, host Host, src string) (Result, error) {
	if host.Shell == nil || host.Adb == nil {
		return Result{}, fmt.Errorf("脚本执行器未就绪")
	}
	if host.Log == nil {
		host.Log = func(string, string) {}
	}
	lines := normalize(src)
	st := &state{
		host:   host,
		vars:   map[string]string{},
		labels: map[string]int{},
		echo:   true,
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ":") && !strings.HasPrefix(trimmed, "::") {
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, ":")))
			if _, exists := st.labels[name]; !exists {
				st.labels[name] = i
			}
		}
	}

	pc := 0
	for pc >= 0 && pc < len(lines) {
		if err := ctx.Err(); err != nil {
			return Result{Steps: st.steps, Failures: st.failures}, fmt.Errorf("脚本已中止")
		}
		st.steps++
		if st.steps > maxSteps {
			return Result{Steps: st.steps, Failures: st.failures}, fmt.Errorf("脚本执行步数超过 %d，可能存在死循环", maxSteps)
		}

		raw := strings.TrimSpace(lines[pc])
		pc++
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "::") {
			continue
		}
		raw = strings.TrimPrefix(raw, "@")
		lower := strings.ToLower(raw)

		switch {
		case lower == "echo off":
			st.echo = false
			continue
		case lower == "echo on":
			st.echo = true
			continue
		}

		next, err := st.exec(ctx, raw, lines, pc)
		if err != nil {
			st.failure(err.Error())
			st.errlevel = 1
		}
		if next >= 0 {
			pc = next
		}
	}
	return Result{Steps: st.steps, Failures: st.failures}, nil
}

// exec 执行一条命令，返回下一行的位置（-1 表示顺序继续）。
func (s *state) exec(ctx context.Context, raw string, lines []string, pc int) (int, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return -1, nil
	}
	cmd := strings.ToLower(fields[0])
	if strings.HasPrefix(cmd, ":") {
		// 标签行，顺序执行到时直接跳过
		return -1, nil
	}

	switch cmd {
	case "rem":
		return -1, nil
	case "echo":
		body := strings.TrimSpace(raw[4:])
		if body == "." {
			body = ""
		}
		s.print(s.expand(body))
		return -1, nil
	case "set":
		return -1, s.doSet(raw)
	case "goto":
		if len(fields) < 2 {
			return -1, fmt.Errorf("goto 缺少标签")
		}
		return s.jump(fields[1])
	case "if":
		return s.doIf(ctx, raw, lines, pc)
	case "for":
		return s.doFor(ctx, raw, lines, pc)
	case "timeout", "choice":
		return -1, s.doSleep(ctx, fields)
	case "pause":
		s.host.Log("info", "已跳过 pause（无人值守执行）")
		return -1, nil
	case "exit":
		return len(lines), nil
	case "call":
		s.host.Log("warn", "不支持 call，已跳过："+raw)
		return -1, nil
	case "cls":
		s.host.Log("info", "cls")
		return -1, nil
	case "adb":
		return -1, s.doAdb(ctx, raw)
	case "cd", "pushd", "popd", "title", "color", "mode", "chcp", "ver", "vol":
		s.host.Log("info", "已忽略本地命令："+raw)
		return -1, nil
	default:
		return -1, fmt.Errorf("不支持的命令：%s", fields[0])
	}
}

func (s *state) doSet(raw string) error {
	body := strings.TrimSpace(raw[3:])
	if body == "" {
		for k, v := range s.vars {
			s.print(k + "=" + v)
		}
		return nil
	}
	lower := strings.ToLower(body)
	if strings.HasPrefix(lower, "/a ") {
		expr := strings.TrimSpace(body[3:])
		name, value, ok := strings.Cut(expr, "=")
		if !ok {
			return fmt.Errorf("set /a 语法错误：%s", expr)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("set /a 缺少变量名")
		}
		v, err := evalExpr(s.expand(strings.TrimSpace(value)))
		if err != nil {
			return err
		}
		s.vars[name] = strconv.FormatInt(v, 10)
		return nil
	}
	if strings.HasPrefix(lower, "/p") {
		return fmt.Errorf("不支持 set /p（需要交互输入）")
	}
	body = strings.Trim(body, "\"")
	k, v, ok := strings.Cut(body, "=")
	if !ok {
		return fmt.Errorf("set 语法错误：%s", body)
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return fmt.Errorf("set 缺少变量名")
	}
	if v == "" {
		delete(s.vars, k)
		return nil
	}
	s.vars[k] = s.expand(strings.TrimSpace(v))
	return nil
}

func (s *state) doIf(ctx context.Context, raw string, lines []string, pc int) (int, error) {
	rest := strings.TrimSpace(raw[2:])
	negate := false
	if strings.HasPrefix(strings.ToLower(rest), "not ") {
		negate = true
		rest = strings.TrimSpace(rest[4:])
	}
	// /i 忽略大小写
	ignoreCase := false
	if strings.HasPrefix(strings.ToLower(rest), "/i ") {
		ignoreCase = true
		rest = strings.TrimSpace(rest[3:])
	}

	var cond bool
	var body string

	lower := strings.ToLower(rest)
	switch {
	case strings.HasPrefix(lower, "errorlevel "):
		rest = strings.TrimSpace(rest[len("errorlevel "):])
		numStr, tail := splitFirst(rest)
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return -1, fmt.Errorf("if errorlevel 数值无效：%s", numStr)
		}
		cond = s.errlevel >= n
		body = tail
	case strings.HasPrefix(lower, "exist "):
		rest = strings.TrimSpace(rest[len("exist "):])
		path, tail := splitFirst(rest)
		path = s.expand(strings.Trim(path, "\""))
		_, err := os.Stat(path)
		cond = err == nil
		body = tail
	default:
		left, tail, ok := cutComparison(rest)
		if !ok {
			return -1, fmt.Errorf("无法解析的 if 条件：%s", rest)
		}
		right, tail2 := splitFirst(tail)
		a := strings.Trim(s.expand(strings.TrimSpace(left)), "\"")
		b := strings.Trim(s.expand(strings.TrimSpace(right)), "\"")
		if ignoreCase {
			cond = strings.EqualFold(a, b)
		} else {
			cond = a == b
		}
		body = tail2
	}

	if negate {
		cond = !cond
	}

	body = strings.TrimSpace(body)
	// 块形式
	if body == "(" || strings.HasPrefix(body, "(") {
		firstRest := strings.TrimSpace(strings.TrimPrefix(body, "("))
		block, next, elseBlock, err := collectBlock(lines, pc, firstRest)
		if err != nil {
			return -1, err
		}
		if cond {
			if err := s.execBlock(ctx, block); err != nil {
				s.failure(err.Error())
			}
			return next, nil
		}
		if len(elseBlock) > 0 {
			return next, s.execBlock(ctx, elseBlock)
		}
		return next, nil
	}
	if strings.HasPrefix(strings.ToLower(body), "else ") {
		body = strings.TrimSpace(body[5:])
		if cond {
			return -1, nil
		}
		return s.execOne(ctx, body)
	}
	if body == "" {
		return -1, fmt.Errorf("if 缺少要执行的命令")
	}
	if !cond {
		return -1, nil
	}
	return s.execOne(ctx, body)
}

func (s *state) doFor(ctx context.Context, raw string, lines []string, pc int) (int, error) {
	rest := strings.TrimSpace(raw[3:])
	if !strings.HasPrefix(strings.ToLower(rest), "/l ") {
		return -1, fmt.Errorf("仅支持 for /l 与 for %%%%i in (..) 形式")
	}
	rest = strings.TrimSpace(rest[3:])
	// %%i in (start,step,end) do body
	varTok, afterVar := cutToken(rest)
	if varTok == "" {
		return -1, fmt.Errorf("for 语法错误")
	}
	varName := strings.TrimSpace(strings.TrimPrefix(varTok, "%%"))
	varName = strings.TrimSuffix(varName, "%")
	if varName == "" {
		return -1, fmt.Errorf("for 缺少循环变量")
	}
	lower := strings.ToLower(afterVar)
	idx := strings.Index(lower, "in (")
	if idx < 0 {
		return -1, fmt.Errorf("for 缺少 in (..)")
	}
	body := afterVar[idx+len("in ("):]
	end := strings.Index(body, ")")
	if end < 0 {
		return -1, fmt.Errorf("for 括号未闭合")
	}
	spec := body[:end]
	tail := strings.TrimSpace(body[end+1:])
	tailLower := strings.ToLower(tail)
	if !strings.HasPrefix(tailLower, "do ") {
		return -1, fmt.Errorf("for 缺少 do")
	}
	bodyPart := strings.TrimSpace(tail[3:])

	parts := strings.Split(spec, ",")
	if len(parts) != 3 {
		return -1, fmt.Errorf("for /l 参数应为 (起始,步长,结束)")
	}
	start, err1 := strconv.Atoi(s.expand(strings.TrimSpace(parts[0])))
	step, err2 := strconv.Atoi(s.expand(strings.TrimSpace(parts[1])))
	endV, err3 := strconv.Atoi(s.expand(strings.TrimSpace(parts[2])))
	if err1 != nil || err2 != nil || err3 != nil {
		return -1, fmt.Errorf("for /l 参数必须是整数")
	}
	if step == 0 {
		return -1, fmt.Errorf("for /l 步长不能为 0")
	}

	var block []string
	next := pc
	if bodyPart == "(" || strings.HasPrefix(bodyPart, "(") {
		firstRest := strings.TrimSpace(strings.TrimPrefix(bodyPart, "("))
		b, n, _, err := collectBlock(lines, pc, firstRest)
		if err != nil {
			return -1, err
		}
		block = b
		next = n
	}

	iter := 0
	for i := start; (step > 0 && i <= endV) || (step < 0 && i >= endV); i += step {
		if err := ctx.Err(); err != nil {
			return -1, fmt.Errorf("脚本已中止")
		}
		iter++
		if iter > maxLoopIter {
			return -1, fmt.Errorf("for 循环次数超过 %d", maxLoopIter)
		}
		s.vars[varName] = strconv.Itoa(i)
		if block != nil {
			if err := s.execBlock(ctx, block); err != nil {
				s.failure(err.Error())
			}
			continue
		}
		if n, err := s.execOne(ctx, bodyPart); err != nil {
			s.failure(err.Error())
		} else if n >= 0 {
			return n, nil
		}
	}
	delete(s.vars, varName)
	if block != nil {
		return next, nil
	}
	return -1, nil
}

func (s *state) execBlock(ctx context.Context, block []string) error {
	for pc := 0; pc < len(block); {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("脚本已中止")
		}
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block[pc]), "@"))
		pc++
		if line == "" || strings.HasPrefix(line, "::") || strings.HasPrefix(strings.ToLower(line), "rem ") {
			continue
		}
		next, err := s.exec(ctx, line, block, pc)
		if err != nil {
			s.errlevel = 1
			s.failure(err.Error())
		}
		if next >= 0 {
			pc = next
		}
	}
	return nil
}

// execOne 执行单行命令，返回跳转目标（-1 表示顺序继续）。
func (s *state) execOne(ctx context.Context, line string) (int, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return -1, nil
	}
	if strings.HasSuffix(line, "(") {
		return -1, fmt.Errorf("单行命令不支持块形式：%s", line)
	}
	next, err := s.exec(ctx, line, nil, 0)
	if err != nil {
		s.errlevel = 1
		return -1, err
	}
	return next, nil
}

func (s *state) doSleep(ctx context.Context, fields []string) error {
	sec := 1
	for i := 1; i < len(fields); i++ {
		f := strings.ToLower(fields[i])
		if f == "/t" && i+1 < len(fields) {
			if n, err := strconv.Atoi(fields[i+1]); err == nil {
				sec = n
			}
		}
	}
	if sec < 0 {
		sec = 0
	}
	if sec > maxSleepSec {
		sec = maxSleepSec
	}
	if sec == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("脚本已中止")
	case <-time.After(time.Duration(sec) * time.Second):
		return nil
	}
}

func (s *state) doAdb(ctx context.Context, raw string) error {
	args := tokenize(s.expand(raw))
	if len(args) < 2 {
		return fmt.Errorf("adb 缺少子命令")
	}
	args = args[1:]
	s.host.Log("cmd", "adb "+strings.Join(args, " "))

	var out string
	var err error
	if strings.EqualFold(args[0], "shell") {
		out, err = s.host.Shell(ctx, strings.Join(args[1:], " "))
	} else {
		out, err = s.host.Adb(ctx, args)
	}
	if strings.TrimSpace(out) != "" {
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			s.print(line)
		}
	}
	if err != nil {
		s.errlevel = 1
		return err
	}
	s.errlevel = 0
	return nil
}

func (s *state) jump(label string) (int, error) {
	label = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(label, ":")))
	if label == "eof" {
		return -1, nil
	}
	if idx, ok := s.labels[label]; ok {
		return idx, nil
	}
	return -1, fmt.Errorf("找不到标签：%s", label)
}

func (s *state) expand(in string) string {
	if in == "" {
		return ""
	}
	out := strings.ReplaceAll(in, "%~dp0", "")
	// for 循环变量写作 %%i
	out = doublePattern.ReplaceAllStringFunc(out, func(m string) string {
		name := m[2:]
		if v, ok := s.vars[name]; ok {
			return v
		}
		for k, v := range s.vars {
			if strings.EqualFold(k, name) {
				return v
			}
		}
		return m
	})
	for i := 0; i < 5; i++ {
		replaced := varPattern.ReplaceAllStringFunc(out, func(m string) string {
			name := m[1 : len(m)-1]
			if strings.EqualFold(name, "errorlevel") {
				return strconv.Itoa(s.errlevel)
			}
			if v, ok := s.vars[name]; ok {
				return v
			}
			// 大小写不敏感回退
			for k, v := range s.vars {
				if strings.EqualFold(k, name) {
					return v
				}
			}
			return ""
		})
		if replaced == out {
			break
		}
		out = replaced
	}
	return out
}

func (s *state) print(text string) {
	s.host.Log("out", text)
}

func (s *state) failure(msg string) {
	s.failures++
	s.host.Log("err", msg)
}

func normalize(src string) []string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return lines
}

// collectBlock 从 pc 行开始收集括号块，返回块内容、下一行位置与 else 块。
func collectBlock(lines []string, pc int, firstRest string) ([]string, int, []string, error) {
	depth := 1
	block := make([]string, 0, 8)
	if firstRest != "" {
		if firstRest == ")" {
			depth = 0
		} else {
			block = append(block, firstRest)
		}
	}
	i := pc
	var elseBlock []string
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		i++
		if depth == 0 {
			break
		}
		if line == ")" {
			depth--
			if depth == 0 {
				break
			}
			block = append(block, line)
			continue
		}
		if strings.HasSuffix(line, ") else (") {
			block = append(block, strings.TrimSpace(strings.TrimSuffix(line, ") else (")))
			depth--
			// 收集 else 块内容
			for i < len(lines) {
				el := strings.TrimSpace(lines[i])
				i++
				if el == ")" {
					break
				}
				elseBlock = append(elseBlock, el)
			}
			break
		}
		block = append(block, line)
	}
	if depth != 0 {
		return nil, 0, nil, fmt.Errorf("括号块未闭合")
	}
	return block, i, elseBlock, nil
}

// splitFirst 取出第一个空白分隔的 token 与剩余部分。
func splitFirst(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if s[0] == '"' {
		if end := strings.Index(s[1:], "\""); end >= 0 {
			return s[:end+2], strings.TrimSpace(s[end+2:])
		}
	}
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i], strings.TrimSpace(s[i:])
		}
	}
	return s, ""
}

// cutToken 取第一个 token，剩余部分连在一起。
func cutToken(s string) (string, string) {
	return splitFirst(s)
}

// cutComparison 在 if "a"=="b" 中切出左右值。
func cutComparison(s string) (string, string, bool) {
	idx := strings.Index(s, "==")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+2:]), true
}

// tokenize 按空白切分，支持双引号包裹。
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// evalExpr 计算 set /a 的整数表达式。
func evalExpr(s string) (int64, error) {
	p := &exprParser{s: []rune(strings.TrimSpace(s))}
	v, err := p.parseAdd()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos < len(p.s) {
		return 0, fmt.Errorf("表达式无法解析：%s", s)
	}
	return v, nil
}

type exprParser struct {
	s   []rune
	pos int
}

func (p *exprParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) parseAdd() (int64, error) {
	v, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return v, nil
		}
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			return v, nil
		}
		p.pos++
		rhs, err := p.parseMul()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += rhs
		} else {
			v -= rhs
		}
	}
}

func (p *exprParser) parseMul() (int64, error) {
	v, err := p.parseAtom()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return v, nil
		}
		op := p.s[p.pos]
		if op != '*' && op != '/' && op != '%' {
			return v, nil
		}
		p.pos++
		rhs, err := p.parseAtom()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			v *= rhs
		case '/':
			if rhs == 0 {
				return 0, fmt.Errorf("除数为 0")
			}
			v /= rhs
		case '%':
			if rhs == 0 {
				return 0, fmt.Errorf("除数为 0")
			}
			v %= rhs
		}
	}
}

func (p *exprParser) parseAtom() (int64, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("表达式不完整")
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, err := p.parseAdd()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("括号未闭合")
		}
		p.pos++
		return v, nil
	}
	start := p.pos
	for p.pos < len(p.s) && (p.s[p.pos] >= '0' && p.s[p.pos] <= '9') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("表达式包含非法字符：%c", p.s[p.pos])
	}
	n, err := strconv.ParseInt(string(p.s[start:p.pos]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("数值超出范围")
	}
	return n, nil
}
