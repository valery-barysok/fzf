package fzf

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/tui"

	"github.com/junegunn/go-shellwords"
)

const usage = `usage: fzf [options]

  Search
    -x, --extended        Extended-search mode
                          (enabled by default; +x or --no-extended to disable)
    -e, --exact           Enable Exact-match
    --algo=TYPE           Fuzzy matching algorithm: [v1|v2] (default: v2)
    -i                    Case-insensitive match (default: smart-case match)
    +i                    Case-sensitive match
    -n, --nth=N[,..]      Comma-separated list of field index expressions
                          for limiting search scope. Each can be a non-zero
                          integer or a range expression ([BEGIN]..[END]).
    --with-nth=N[,..]     Transform the presentation of each line using
                          field index expressions
    -d, --delimiter=STR   Field delimiter regex (default: AWK-style)
    +s, --no-sort         Do not sort the result
    --tac                 Reverse the order of the input
    --tiebreak=CRI[,..]   Comma-separated list of sort criteria to apply
                          when the scores are tied [length|begin|end|index]
                          (default: length)

  Interface
    -m, --multi           Enable multi-select with tab/shift-tab
    --no-mouse            Disable mouse
    --bind=KEYBINDS       Custom key bindings. Refer to the man page.
    --cycle               Enable cyclic scroll
    --no-hscroll          Disable horizontal scroll
    --hscroll-off=COL     Number of screen columns to keep to the right of the
                          highlighted substring (default: 10)
    --jump-labels=CHARS   Label characters for jump and jump-accept

  Layout
    --reverse             Reverse orientation
    --margin=MARGIN       Screen margin (TRBL / TB,RL / T,RL,B / T,R,B,L)
    --inline-info         Display finder info inline with the query
    --prompt=STR          Input prompt (default: '> ')
    --header=STR          String to print as header
    --header-lines=N      The first N lines of the input are treated as header

  Display
    --ansi                Enable processing of ANSI color codes
    --tabstop=SPACES      Number of spaces for a tab character (default: 8)
    --color=COLSPEC       Base scheme (dark|light|16|bw) and/or custom colors
    --no-bold             Do not use bold text

  History
    --history=FILE        History file
    --history-size=N      Maximum number of history entries (default: 1000)

  Preview
    --preview=COMMAND     Command to preview highlighted line ({})
    --preview-window=OPT  Preview window layout (default: right:50%)
                          [up|down|left|right][:SIZE[%]][:hidden]

  Scripting
    -q, --query=STR       Start the finder with the given query
    -1, --select-1        Automatically select the only match
    -0, --exit-0          Exit immediately when there's no match
    -f, --filter=STR      Filter mode. Do not start interactive finder.
    --print-query         Print query as the first line
    --expect=KEYS         Comma-separated list of keys to complete fzf
    --sync                Synchronous search for multi-staged filtering

  Environment variables
    FZF_DEFAULT_COMMAND   Default command to use when input is tty
    FZF_DEFAULT_OPTS      Default options (e.g. '--reverse --inline-info')

`

// Case denotes case-sensitivity of search
type Case int

// Case-sensitivities
const (
	CaseSmart Case = iota
	CaseIgnore
	CaseRespect
)

// Sort criteria
type criterion int

const (
	byScore criterion = iota
	byLength
	byBegin
	byEnd
)

type sizeSpec struct {
	size    float64
	percent bool
}

func defaultMargin() [4]sizeSpec {
	return [4]sizeSpec{}
}

type windowPosition int

const (
	posUp windowPosition = iota
	posDown
	posLeft
	posRight
)

type previewOpts struct {
	command  string
	position windowPosition
	size     sizeSpec
	hidden   bool
}

// Options stores the values of command-line options
type Options struct {
	Fuzzy       bool
	FuzzyAlgo   algo.Algo
	Extended    bool
	Case        Case
	Nth         []Range
	WithNth     []Range
	Delimiter   Delimiter
	Sort        int
	Tac         bool
	Criteria    []criterion
	Multi       bool
	Ansi        bool
	Mouse       bool
	Theme       *tui.ColorTheme
	Black       bool
	Bold        bool
	Reverse     bool
	Cycle       bool
	Hscroll     bool
	HscrollOff  int
	InlineInfo  bool
	JumpLabels  string
	Prompt      string
	Query       string
	Select1     bool
	Exit0       bool
	Filter      *string
	ToggleSort  bool
	Expect      map[int]string
	Keymap      map[int]actionType
	Execmap     map[int]string
	Preview     previewOpts
	PrintQuery  bool
	ReadZero    bool
	Printer     func(string)
	Sync        bool
	History     *History
	Header      []string
	HeaderLines int
	Margin      [4]sizeSpec
	Tabstop     int
	Version     bool
}

func defaultOptions() *Options {
	return &Options{
		Fuzzy:       true,
		FuzzyAlgo:   algo.FuzzyMatchV2,
		Extended:    true,
		Case:        CaseSmart,
		Nth:         make([]Range, 0),
		WithNth:     make([]Range, 0),
		Delimiter:   Delimiter{},
		Sort:        1000,
		Tac:         false,
		Criteria:    []criterion{byScore, byLength},
		Multi:       false,
		Ansi:        false,
		Mouse:       true,
		Theme:       tui.EmptyTheme(),
		Black:       false,
		Bold:        true,
		Reverse:     false,
		Cycle:       false,
		Hscroll:     true,
		HscrollOff:  10,
		InlineInfo:  false,
		JumpLabels:  defaultJumpLabels,
		Prompt:      "> ",
		Query:       "",
		Select1:     false,
		Exit0:       false,
		Filter:      nil,
		ToggleSort:  false,
		Expect:      make(map[int]string),
		Keymap:      make(map[int]actionType),
		Execmap:     make(map[int]string),
		Preview:     previewOpts{"", posRight, sizeSpec{50, true}, false},
		PrintQuery:  false,
		ReadZero:    false,
		Printer:     func(str string) { fmt.Println(str) },
		Sync:        false,
		History:     nil,
		Header:      make([]string, 0),
		HeaderLines: 0,
		Margin:      defaultMargin(),
		Tabstop:     8,
		Version:     false}
}

func help(code int) {
	os.Stderr.WriteString(usage)
	os.Exit(code)
}

func errorExit(msg string) {
	os.Stderr.WriteString(msg + "\n")
	os.Exit(exitError)
}

func optString(arg string, prefixes ...string) (bool, string) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(arg, prefix) {
			return true, arg[len(prefix):]
		}
	}
	return false, ""
}

func nextString(args []string, i *int, message string) string {
	if len(args) > *i+1 {
		*i++
	} else {
		errorExit(message)
	}
	return args[*i]
}

func optionalNextString(args []string, i *int) string {
	if len(args) > *i+1 && !strings.HasPrefix(args[*i+1], "-") {
		*i++
		return args[*i]
	}
	return ""
}

func atoi(str string) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		errorExit("not a valid integer: " + str)
	}
	return num
}

func atof(str string) float64 {
	num, err := strconv.ParseFloat(str, 64)
	if err != nil {
		errorExit("not a valid number: " + str)
	}
	return num
}

func nextInt(args []string, i *int, message string) int {
	if len(args) > *i+1 {
		*i++
	} else {
		errorExit(message)
	}
	return atoi(args[*i])
}

func optionalNumeric(args []string, i *int) int {
	if len(args) > *i+1 {
		if strings.IndexAny(args[*i+1], "0123456789") == 0 {
			*i++
		}
	}
	return 1 // Don't care
}

func splitNth(str string) []Range {
	if match, _ := regexp.MatchString("^[0-9,-.]+$", str); !match {
		errorExit("invalid format: " + str)
	}

	tokens := strings.Split(str, ",")
	ranges := make([]Range, len(tokens))
	for idx, s := range tokens {
		r, ok := ParseRange(&s)
		if !ok {
			errorExit("invalid format: " + str)
		}
		ranges[idx] = r
	}
	return ranges
}

func delimiterRegexp(str string) Delimiter {
	// Special handling of \t
	str = strings.Replace(str, "\\t", "\t", -1)

	// 1. Pattern does not contain any special character
	if regexp.QuoteMeta(str) == str {
		return Delimiter{str: &str}
	}

	rx, e := regexp.Compile(str)
	// 2. Pattern is not a valid regular expression
	if e != nil {
		return Delimiter{str: &str}
	}

	// 3. Pattern as regular expression. Slow.
	return Delimiter{regex: rx}
}

func isAlphabet(char uint8) bool {
	return char >= 'a' && char <= 'z'
}

func parseAlgo(str string) algo.Algo {
	switch str {
	case "v1":
		return algo.FuzzyMatchV1
	case "v2":
		return algo.FuzzyMatchV2
	default:
		errorExit("invalid algorithm (expected: v1 or v2)")
	}
	return algo.FuzzyMatchV2
}

func parseKeyChords(str string, message string) map[int]string {
	if len(str) == 0 {
		errorExit(message)
	}

	tokens := strings.Split(str, ",")
	if str == "," || strings.HasPrefix(str, ",,") || strings.HasSuffix(str, ",,") || strings.Index(str, ",,,") >= 0 {
		tokens = append(tokens, ",")
	}

	chords := make(map[int]string)
	for _, key := range tokens {
		if len(key) == 0 {
			continue // ignore
		}
		lkey := strings.ToLower(key)
		chord := 0
		switch lkey {
		case "up":
			chord = tui.Up
		case "down":
			chord = tui.Down
		case "left":
			chord = tui.Left
		case "right":
			chord = tui.Right
		case "enter", "return":
			chord = tui.CtrlM
		case "space":
			chord = tui.AltZ + int(' ')
		case "bspace", "bs":
			chord = tui.BSpace
		case "alt-enter", "alt-return":
			chord = tui.AltEnter
		case "alt-space":
			chord = tui.AltSpace
		case "alt-/":
			chord = tui.AltSlash
		case "alt-bs", "alt-bspace":
			chord = tui.AltBS
		case "tab":
			chord = tui.Tab
		case "btab", "shift-tab":
			chord = tui.BTab
		case "esc":
			chord = tui.ESC
		case "del":
			chord = tui.Del
		case "home":
			chord = tui.Home
		case "end":
			chord = tui.End
		case "pgup", "page-up":
			chord = tui.PgUp
		case "pgdn", "page-down":
			chord = tui.PgDn
		case "shift-left":
			chord = tui.SLeft
		case "shift-right":
			chord = tui.SRight
		case "double-click":
			chord = tui.DoubleClick
		case "f10":
			chord = tui.F10
		default:
			if len(key) == 6 && strings.HasPrefix(lkey, "ctrl-") && isAlphabet(lkey[5]) {
				chord = tui.CtrlA + int(lkey[5]) - 'a'
			} else if len(key) == 5 && strings.HasPrefix(lkey, "alt-") && isAlphabet(lkey[4]) {
				chord = tui.AltA + int(lkey[4]) - 'a'
			} else if len(key) == 2 && strings.HasPrefix(lkey, "f") && key[1] >= '1' && key[1] <= '9' {
				chord = tui.F1 + int(key[1]) - '1'
			} else if utf8.RuneCountInString(key) == 1 {
				chord = tui.AltZ + int([]rune(key)[0])
			} else {
				errorExit("unsupported key: " + key)
			}
		}
		if chord > 0 {
			chords[chord] = key
		}
	}
	return chords
}

func parseTiebreak(str string) []criterion {
	criteria := []criterion{byScore}
	hasIndex := false
	hasLength := false
	hasBegin := false
	hasEnd := false
	check := func(notExpected *bool, name string) {
		if *notExpected {
			errorExit("duplicate sort criteria: " + name)
		}
		if hasIndex {
			errorExit("index should be the last criterion")
		}
		*notExpected = true
	}
	for _, str := range strings.Split(strings.ToLower(str), ",") {
		switch str {
		case "index":
			check(&hasIndex, "index")
		case "length":
			check(&hasLength, "length")
			criteria = append(criteria, byLength)
		case "begin":
			check(&hasBegin, "begin")
			criteria = append(criteria, byBegin)
		case "end":
			check(&hasEnd, "end")
			criteria = append(criteria, byEnd)
		default:
			errorExit("invalid sort criterion: " + str)
		}
	}
	return criteria
}

func dupeTheme(theme *tui.ColorTheme) *tui.ColorTheme {
	if theme != nil {
		dupe := *theme
		return &dupe
	}
	return nil
}

func parseTheme(defaultTheme *tui.ColorTheme, str string) *tui.ColorTheme {
	theme := dupeTheme(defaultTheme)
	for _, str := range strings.Split(strings.ToLower(str), ",") {
		switch str {
		case "dark":
			theme = dupeTheme(tui.Dark256)
		case "light":
			theme = dupeTheme(tui.Light256)
		case "16":
			theme = dupeTheme(tui.Default16)
		case "bw", "no":
			theme = nil
		default:
			fail := func() {
				errorExit("invalid color specification: " + str)
			}
			// Color is disabled
			if theme == nil {
				continue
			}

			pair := strings.Split(str, ":")
			if len(pair) != 2 {
				fail()
			}
			ansi32, err := strconv.Atoi(pair[1])
			if err != nil || ansi32 < -1 || ansi32 > 255 {
				fail()
			}
			ansi := tui.Color(ansi32)
			switch pair[0] {
			case "fg":
				theme.Fg = ansi
			case "bg":
				theme.Bg = ansi
			case "fg+":
				theme.Current = ansi
			case "bg+":
				theme.DarkBg = ansi
			case "hl":
				theme.Match = ansi
			case "hl+":
				theme.CurrentMatch = ansi
			case "border":
				theme.Border = ansi
			case "prompt":
				theme.Prompt = ansi
			case "spinner":
				theme.Spinner = ansi
			case "info":
				theme.Info = ansi
			case "pointer":
				theme.Cursor = ansi
			case "marker":
				theme.Selected = ansi
			case "header":
				theme.Header = ansi
			default:
				fail()
			}
		}
	}
	return theme
}

var executeRegexp *regexp.Regexp

func firstKey(keymap map[int]string) int {
	for k := range keymap {
		return k
	}
	return 0
}

const (
	escapedColon = 0
	escapedComma = 1
)

func parseKeymap(keymap map[int]actionType, execmap map[int]string, str string) {
	if executeRegexp == nil {
		// Backreferences are not supported.
		// "~!@#$%^&*;/|".each_char.map { |c| Regexp.escape(c) }.map { |c| "#{c}[^#{c}]*#{c}" }.join('|')
		executeRegexp = regexp.MustCompile(
			"(?s):execute(-multi)?:.*|:execute(-multi)?(\\([^)]*\\)|\\[[^\\]]*\\]|~[^~]*~|![^!]*!|@[^@]*@|\\#[^\\#]*\\#|\\$[^\\$]*\\$|%[^%]*%|\\^[^\\^]*\\^|&[^&]*&|\\*[^\\*]*\\*|;[^;]*;|/[^/]*/|\\|[^\\|]*\\|)")
	}
	masked := executeRegexp.ReplaceAllStringFunc(str, func(src string) string {
		if strings.HasPrefix(src, ":execute-multi") {
			return ":execute-multi(" + strings.Repeat(" ", len(src)-len(":execute-multi()")) + ")"
		}
		return ":execute(" + strings.Repeat(" ", len(src)-len(":execute()")) + ")"
	})
	masked = strings.Replace(masked, "::", string([]rune{escapedColon, ':'}), -1)
	masked = strings.Replace(masked, ",:", string([]rune{escapedComma, ':'}), -1)

	idx := 0
	for _, pairStr := range strings.Split(masked, ",") {
		origPairStr := str[idx : idx+len(pairStr)]
		idx += len(pairStr) + 1

		pair := strings.SplitN(pairStr, ":", 2)
		if len(pair) < 2 {
			errorExit("bind action not specified: " + origPairStr)
		}
		var key int
		if len(pair[0]) == 1 && pair[0][0] == escapedColon {
			key = ':' + tui.AltZ
		} else if len(pair[0]) == 1 && pair[0][0] == escapedComma {
			key = ',' + tui.AltZ
		} else {
			keys := parseKeyChords(pair[0], "key name required")
			key = firstKey(keys)
		}

		act := origPairStr[len(pair[0])+1 : len(origPairStr)]
		actLower := strings.ToLower(act)
		switch actLower {
		case "ignore":
			keymap[key] = actIgnore
		case "beginning-of-line":
			keymap[key] = actBeginningOfLine
		case "abort":
			keymap[key] = actAbort
		case "accept":
			keymap[key] = actAccept
		case "print-query":
			keymap[key] = actPrintQuery
		case "backward-char":
			keymap[key] = actBackwardChar
		case "backward-delete-char":
			keymap[key] = actBackwardDeleteChar
		case "backward-word":
			keymap[key] = actBackwardWord
		case "clear-screen":
			keymap[key] = actClearScreen
		case "delete-char":
			keymap[key] = actDeleteChar
		case "delete-char/eof":
			keymap[key] = actDeleteCharEOF
		case "end-of-line":
			keymap[key] = actEndOfLine
		case "cancel":
			keymap[key] = actCancel
		case "forward-char":
			keymap[key] = actForwardChar
		case "forward-word":
			keymap[key] = actForwardWord
		case "jump":
			keymap[key] = actJump
		case "jump-accept":
			keymap[key] = actJumpAccept
		case "kill-line":
			keymap[key] = actKillLine
		case "kill-word":
			keymap[key] = actKillWord
		case "unix-line-discard", "line-discard":
			keymap[key] = actUnixLineDiscard
		case "unix-word-rubout", "word-rubout":
			keymap[key] = actUnixWordRubout
		case "yank":
			keymap[key] = actYank
		case "backward-kill-word":
			keymap[key] = actBackwardKillWord
		case "toggle-down":
			keymap[key] = actToggleDown
		case "toggle-up":
			keymap[key] = actToggleUp
		case "toggle-in":
			keymap[key] = actToggleIn
		case "toggle-out":
			keymap[key] = actToggleOut
		case "toggle-all":
			keymap[key] = actToggleAll
		case "select-all":
			keymap[key] = actSelectAll
		case "deselect-all":
			keymap[key] = actDeselectAll
		case "toggle":
			keymap[key] = actToggle
		case "down":
			keymap[key] = actDown
		case "up":
			keymap[key] = actUp
		case "page-up":
			keymap[key] = actPageUp
		case "page-down":
			keymap[key] = actPageDown
		case "previous-history":
			keymap[key] = actPreviousHistory
		case "next-history":
			keymap[key] = actNextHistory
		case "toggle-preview":
			keymap[key] = actTogglePreview
		case "toggle-sort":
			keymap[key] = actToggleSort
		case "preview-up":
			keymap[key] = actPreviewUp
		case "preview-down":
			keymap[key] = actPreviewDown
		case "preview-page-up":
			keymap[key] = actPreviewPageUp
		case "preview-page-down":
			keymap[key] = actPreviewPageDown
		default:
			if isExecuteAction(actLower) {
				var offset int
				if strings.HasPrefix(actLower, "execute-multi") {
					keymap[key] = actExecuteMulti
					offset = len("execute-multi")
				} else {
					keymap[key] = actExecute
					offset = len("execute")
				}
				if act[offset] == ':' {
					execmap[key] = act[offset+1:]
				} else {
					execmap[key] = act[offset+1 : len(act)-1]
				}
			} else {
				errorExit("unknown action: " + act)
			}
		}
	}
}

func isExecuteAction(str string) bool {
	if !strings.HasPrefix(str, "execute") || len(str) < len("execute()") {
		return false
	}
	b := str[len("execute")]
	if strings.HasPrefix(str, "execute-multi") {
		if len(str) < len("execute-multi()") {
			return false
		}
		b = str[len("execute-multi")]
	}
	e := str[len(str)-1]
	if b == ':' || b == '(' && e == ')' || b == '[' && e == ']' ||
		b == e && strings.ContainsAny(string(b), "~!@#$%^&*;/|") {
		return true
	}
	return false
}

func parseToggleSort(keymap map[int]actionType, str string) {
	keys := parseKeyChords(str, "key name required")
	if len(keys) != 1 {
		errorExit("multiple keys specified")
	}
	keymap[firstKey(keys)] = actToggleSort
}

func strLines(str string) []string {
	return strings.Split(strings.TrimSuffix(str, "\n"), "\n")
}

func parseSize(str string, maxPercent float64, label string) sizeSpec {
	var val float64
	percent := strings.HasSuffix(str, "%")
	if percent {
		val = atof(str[:len(str)-1])
		if val < 0 {
			errorExit(label + " must be non-negative")
		}
		if val > maxPercent {
			errorExit(fmt.Sprintf("%s too large (max: %d%%)", label, int(maxPercent)))
		}
	} else {
		if strings.Contains(str, ".") {
			errorExit(label + " (without %) must be a non-negative integer")
		}

		val = float64(atoi(str))
		if val < 0 {
			errorExit(label + " must be non-negative")
		}
	}
	return sizeSpec{val, percent}
}

func parsePreviewWindow(opts *previewOpts, input string) {
	layout := input
	opts.hidden = false
	if strings.HasSuffix(layout, ":hidden") {
		opts.hidden = true
		layout = strings.TrimSuffix(layout, ":hidden")
	}

	tokens := strings.Split(layout, ":")
	if len(tokens) == 0 || len(tokens) > 2 {
		errorExit("invalid window layout: " + input)
	}

	if len(tokens) > 1 {
		opts.size = parseSize(tokens[1], 99, "window size")
	} else {
		opts.size = sizeSpec{50, true}
	}
	if !opts.size.percent && opts.size.size > 0 {
		// Adjust size for border
		opts.size.size += 2
	}

	switch tokens[0] {
	case "up":
		opts.position = posUp
	case "down":
		opts.position = posDown
	case "left":
		opts.position = posLeft
	case "right":
		opts.position = posRight
	default:
		errorExit("invalid window position: " + input)
	}
}

func parseMargin(margin string) [4]sizeSpec {
	margins := strings.Split(margin, ",")
	checked := func(str string) sizeSpec {
		return parseSize(str, 49, "margin")
	}
	switch len(margins) {
	case 1:
		m := checked(margins[0])
		return [4]sizeSpec{m, m, m, m}
	case 2:
		tb := checked(margins[0])
		rl := checked(margins[1])
		return [4]sizeSpec{tb, rl, tb, rl}
	case 3:
		t := checked(margins[0])
		rl := checked(margins[1])
		b := checked(margins[2])
		return [4]sizeSpec{t, rl, b, rl}
	case 4:
		return [4]sizeSpec{
			checked(margins[0]), checked(margins[1]),
			checked(margins[2]), checked(margins[3])}
	default:
		errorExit("invalid margin: " + margin)
	}
	return defaultMargin()
}

func parseOptions(opts *Options, allArgs []string) {
	var historyMax int
	if opts.History == nil {
		historyMax = defaultHistoryMax
	} else {
		historyMax = opts.History.maxSize
	}
	setHistory := func(path string) {
		h, e := NewHistory(path, historyMax)
		if e != nil {
			errorExit(e.Error())
		}
		opts.History = h
	}
	setHistoryMax := func(max int) {
		historyMax = max
		if historyMax < 1 {
			errorExit("history max must be a positive integer")
		}
		if opts.History != nil {
			opts.History.maxSize = historyMax
		}
	}
	validateJumpLabels := false
	for i := 0; i < len(allArgs); i++ {
		arg := allArgs[i]
		switch arg {
		case "-h", "--help":
			help(exitOk)
		case "-x", "--extended":
			opts.Extended = true
		case "-e", "--exact":
			opts.Fuzzy = false
		case "--extended-exact":
			// Note that we now don't have --no-extended-exact
			opts.Fuzzy = false
			opts.Extended = true
		case "+x", "--no-extended":
			opts.Extended = false
		case "+e", "--no-exact":
			opts.Fuzzy = true
		case "-q", "--query":
			opts.Query = nextString(allArgs, &i, "query string required")
		case "-f", "--filter":
			filter := nextString(allArgs, &i, "query string required")
			opts.Filter = &filter
		case "--algo":
			opts.FuzzyAlgo = parseAlgo(nextString(allArgs, &i, "algorithm required (v1|v2)"))
		case "--expect":
			opts.Expect = parseKeyChords(nextString(allArgs, &i, "key names required"), "key names required")
		case "--tiebreak":
			opts.Criteria = parseTiebreak(nextString(allArgs, &i, "sort criterion required"))
		case "--bind":
			parseKeymap(opts.Keymap, opts.Execmap, nextString(allArgs, &i, "bind expression required"))
		case "--color":
			spec := optionalNextString(allArgs, &i)
			if len(spec) == 0 {
				opts.Theme = tui.EmptyTheme()
			} else {
				opts.Theme = parseTheme(opts.Theme, spec)
			}
		case "--toggle-sort":
			parseToggleSort(opts.Keymap, nextString(allArgs, &i, "key name required"))
		case "-d", "--delimiter":
			opts.Delimiter = delimiterRegexp(nextString(allArgs, &i, "delimiter required"))
		case "-n", "--nth":
			opts.Nth = splitNth(nextString(allArgs, &i, "nth expression required"))
		case "--with-nth":
			opts.WithNth = splitNth(nextString(allArgs, &i, "nth expression required"))
		case "-s", "--sort":
			opts.Sort = optionalNumeric(allArgs, &i)
		case "+s", "--no-sort":
			opts.Sort = 0
		case "--tac":
			opts.Tac = true
		case "--no-tac":
			opts.Tac = false
		case "-i":
			opts.Case = CaseIgnore
		case "+i":
			opts.Case = CaseRespect
		case "-m", "--multi":
			opts.Multi = true
		case "+m", "--no-multi":
			opts.Multi = false
		case "--ansi":
			opts.Ansi = true
		case "--no-ansi":
			opts.Ansi = false
		case "--no-mouse":
			opts.Mouse = false
		case "+c", "--no-color":
			opts.Theme = nil
		case "+2", "--no-256":
			opts.Theme = tui.Default16
		case "--black":
			opts.Black = true
		case "--no-black":
			opts.Black = false
		case "--bold":
			opts.Bold = true
		case "--no-bold":
			opts.Bold = false
		case "--reverse":
			opts.Reverse = true
		case "--no-reverse":
			opts.Reverse = false
		case "--cycle":
			opts.Cycle = true
		case "--no-cycle":
			opts.Cycle = false
		case "--hscroll":
			opts.Hscroll = true
		case "--no-hscroll":
			opts.Hscroll = false
		case "--hscroll-off":
			opts.HscrollOff = nextInt(allArgs, &i, "hscroll offset required")
		case "--inline-info":
			opts.InlineInfo = true
		case "--no-inline-info":
			opts.InlineInfo = false
		case "--jump-labels":
			opts.JumpLabels = nextString(allArgs, &i, "label characters required")
			validateJumpLabels = true
		case "-1", "--select-1":
			opts.Select1 = true
		case "+1", "--no-select-1":
			opts.Select1 = false
		case "-0", "--exit-0":
			opts.Exit0 = true
		case "+0", "--no-exit-0":
			opts.Exit0 = false
		case "--read0":
			opts.ReadZero = true
		case "--no-read0":
			opts.ReadZero = false
		case "--print0":
			opts.Printer = func(str string) { fmt.Print(str, "\x00") }
		case "--no-print0":
			opts.Printer = func(str string) { fmt.Println(str) }
		case "--print-query":
			opts.PrintQuery = true
		case "--no-print-query":
			opts.PrintQuery = false
		case "--prompt":
			opts.Prompt = nextString(allArgs, &i, "prompt string required")
		case "--sync":
			opts.Sync = true
		case "--no-sync":
			opts.Sync = false
		case "--async":
			opts.Sync = false
		case "--no-history":
			opts.History = nil
		case "--history":
			setHistory(nextString(allArgs, &i, "history file path required"))
		case "--history-size":
			setHistoryMax(nextInt(allArgs, &i, "history max size required"))
		case "--no-header":
			opts.Header = []string{}
		case "--no-header-lines":
			opts.HeaderLines = 0
		case "--header":
			opts.Header = strLines(nextString(allArgs, &i, "header string required"))
		case "--header-lines":
			opts.HeaderLines = atoi(
				nextString(allArgs, &i, "number of header lines required"))
		case "--preview":
			opts.Preview.command = nextString(allArgs, &i, "preview command required")
		case "--no-preview":
			opts.Preview.command = ""
		case "--preview-window":
			parsePreviewWindow(&opts.Preview,
				nextString(allArgs, &i, "preview window layout required: [up|down|left|right][:SIZE[%]]"))
		case "--no-margin":
			opts.Margin = defaultMargin()
		case "--margin":
			opts.Margin = parseMargin(
				nextString(allArgs, &i, "margin required (TRBL / TB,RL / T,RL,B / T,R,B,L)"))
		case "--tabstop":
			opts.Tabstop = nextInt(allArgs, &i, "tab stop required")
		case "--version":
			opts.Version = true
		default:
			if match, value := optString(arg, "--algo="); match {
				opts.FuzzyAlgo = parseAlgo(value)
			} else if match, value := optString(arg, "-q", "--query="); match {
				opts.Query = value
			} else if match, value := optString(arg, "-f", "--filter="); match {
				opts.Filter = &value
			} else if match, value := optString(arg, "-d", "--delimiter="); match {
				opts.Delimiter = delimiterRegexp(value)
			} else if match, value := optString(arg, "--prompt="); match {
				opts.Prompt = value
			} else if match, value := optString(arg, "-n", "--nth="); match {
				opts.Nth = splitNth(value)
			} else if match, value := optString(arg, "--with-nth="); match {
				opts.WithNth = splitNth(value)
			} else if match, _ := optString(arg, "-s", "--sort="); match {
				opts.Sort = 1 // Don't care
			} else if match, value := optString(arg, "--toggle-sort="); match {
				parseToggleSort(opts.Keymap, value)
			} else if match, value := optString(arg, "--expect="); match {
				opts.Expect = parseKeyChords(value, "key names required")
			} else if match, value := optString(arg, "--tiebreak="); match {
				opts.Criteria = parseTiebreak(value)
			} else if match, value := optString(arg, "--color="); match {
				opts.Theme = parseTheme(opts.Theme, value)
			} else if match, value := optString(arg, "--bind="); match {
				parseKeymap(opts.Keymap, opts.Execmap, value)
			} else if match, value := optString(arg, "--history="); match {
				setHistory(value)
			} else if match, value := optString(arg, "--history-size="); match {
				setHistoryMax(atoi(value))
			} else if match, value := optString(arg, "--header="); match {
				opts.Header = strLines(value)
			} else if match, value := optString(arg, "--header-lines="); match {
				opts.HeaderLines = atoi(value)
			} else if match, value := optString(arg, "--preview="); match {
				opts.Preview.command = value
			} else if match, value := optString(arg, "--preview-window="); match {
				parsePreviewWindow(&opts.Preview, value)
			} else if match, value := optString(arg, "--margin="); match {
				opts.Margin = parseMargin(value)
			} else if match, value := optString(arg, "--tabstop="); match {
				opts.Tabstop = atoi(value)
			} else if match, value := optString(arg, "--hscroll-off="); match {
				opts.HscrollOff = atoi(value)
			} else if match, value := optString(arg, "--jump-labels="); match {
				opts.JumpLabels = value
			} else {
				errorExit("unknown option: " + arg)
			}
		}
	}

	if opts.HeaderLines < 0 {
		errorExit("header lines must be a non-negative integer")
	}

	if opts.HscrollOff < 0 {
		errorExit("hscroll offset must be a non-negative integer")
	}

	if opts.Tabstop < 1 {
		errorExit("tab stop must be a positive integer")
	}

	if len(opts.JumpLabels) == 0 {
		errorExit("empty jump labels")
	}

	if validateJumpLabels {
		for _, r := range opts.JumpLabels {
			if r < 32 || r > 126 {
				errorExit("non-ascii jump labels are not allowed")
			}
		}
	}
}

func postProcessOptions(opts *Options) {
	// Default actions for CTRL-N / CTRL-P when --history is set
	if opts.History != nil {
		if _, prs := opts.Keymap[tui.CtrlP]; !prs {
			opts.Keymap[tui.CtrlP] = actPreviousHistory
		}
		if _, prs := opts.Keymap[tui.CtrlN]; !prs {
			opts.Keymap[tui.CtrlN] = actNextHistory
		}
	}

	// Extend the default key map
	keymap := defaultKeymap()
	for key, act := range opts.Keymap {
		if act == actToggleSort {
			opts.ToggleSort = true
		}
		keymap[key] = act
	}
	opts.Keymap = keymap

	// If we're not using extended search mode, --nth option becomes irrelevant
	// if it contains the whole range
	if !opts.Extended || len(opts.Nth) == 1 {
		for _, r := range opts.Nth {
			if r.begin == rangeEllipsis && r.end == rangeEllipsis {
				opts.Nth = make([]Range, 0)
				return
			}
		}
	}
}

// ParseOptions parses command-line options
func ParseOptions() *Options {
	opts := defaultOptions()

	// Options from Env var
	words, _ := shellwords.Parse(os.Getenv("FZF_DEFAULT_OPTS"))
	if len(words) > 0 {
		parseOptions(opts, words)
	}

	// Options from command-line arguments
	parseOptions(opts, os.Args[1:])

	postProcessOptions(opts)
	return opts
}
