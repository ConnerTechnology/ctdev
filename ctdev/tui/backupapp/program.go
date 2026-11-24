package backupapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// frameWidth is the widest a screen is drawn.
const frameWidth = 96

// Scheduler installs and removes this computer's hourly runner; cmd wires it
// to the backup package, and tests to a fake.
type Scheduler interface {
	Install(ctx context.Context) error
	RemoveIfIdle(ctx context.Context) (bool, error)
}

// Deps is what the flows run on.
type Deps struct {
	Machine   *backup.Machine
	Cwd       string
	Scheduler Scheduler
}

// programOptions are the terminal the program runs in; tests replace it.
var programOptions = func() []tea.ProgramOption { return nil }

// RunFolder runs `ctdev backup` for the folder at d.Cwd.
func RunFolder(ctx context.Context, d Deps) error {
	return run(ctx, d, os.Stdout, folderFlow)
}

// RunAll runs `ctdev backup --all`.
func RunAll(ctx context.Context, d Deps) error {
	return run(ctx, d, os.Stdout, allFlow)
}

// run shows the program while flow runs on its own goroutine. Once the
// program exits it cancels the flow and waits for it, so no restic outlives
// it, then prints the result lines to out.
func run(ctx context.Context, d Deps, out io.Writer, flow func(context.Context, UI, Deps) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	msgs := make(chan tea.Msg)
	done := make(chan struct{})
	c := &client{out: msgs, done: done}
	m := &model{in: msgs, done: done, dryRun: d.Machine.Opts.DryRun}

	saved := d.Machine.Opts.Stdout
	d.Machine.Opts.Stdout = &lineWriter{post: func(line string) { c.post(logMsg(line)) }}
	defer func() { d.Machine.Opts.Stdout = saved }()

	var flowErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		flowErr = flow(ctx, c, d)
		c.post(flowDoneMsg{})
	})
	p := tea.NewProgram(m, append([]tea.ProgramOption{tea.WithContext(ctx)}, programOptions()...)...)
	_, err := p.Run()
	close(done)
	cancel()
	wg.Wait()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		err = nil
	}
	for _, l := range c.said() {
		fmt.Fprintln(out, "  "+styleLine(l))
	}
	if errors.Is(flowErr, ErrCancelled) || errors.Is(flowErr, ErrBack) {
		flowErr = nil
	}
	return errors.Join(err, flowErr)
}

// Messages from the flow's goroutine, all on one channel so they arrive in
// the order they were sent.
type (
	request struct {
		page  Page
		step  step
		reply chan answer // nil for a running step, which isn't answered
	}
	sayMsg      Line
	logMsg      string
	progressMsg Progress
	flowDoneMsg struct{}
	answer      struct {
		value any
		err   error
	}
)

// client is the flow's side of the UI.
type client struct {
	out  chan<- tea.Msg
	done <-chan struct{} // closed once the program has exited

	mu    sync.Mutex
	lines []Line
}

// post sends msg unless the program has exited.
func (c *client) post(msg tea.Msg) {
	select {
	case c.out <- msg:
	case <-c.done:
	}
}

func (c *client) send(ctx context.Context, msg tea.Msg) error {
	select {
	case c.out <- msg:
		return nil
	case <-ctx.Done():
	case <-c.done:
	}
	return ErrCancelled
}

// ask shows a step and waits for its answer.
func (c *client) ask(ctx context.Context, page Page, s step) (any, error) {
	reply := make(chan answer, 1)
	if err := c.send(ctx, request{page: page, step: s, reply: reply}); err != nil {
		return nil, err
	}
	select {
	case a := <-reply:
		return a.value, a.err
	case <-ctx.Done():
	case <-c.done:
	}
	return nil, ErrCancelled
}

func (c *client) said() []Line {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lines
}

func (c *client) Say(kind Kind, text string) {
	c.mu.Lock()
	c.lines = append(c.lines, Line{kind, text})
	c.mu.Unlock()
	c.post(sayMsg{kind, text})
}

func (c *client) Dashboard(ctx context.Context, d Dashboard) (Action, error) {
	v, err := c.ask(ctx, d.Page, newDashboard(d))
	a, _ := v.(Action)
	return a, err
}

func (c *client) Pick(ctx context.Context, p Picker) (int, error) {
	v, err := c.ask(ctx, p.Page, newPicker(p))
	i, _ := v.(int)
	return i, err
}

func (c *client) Confirm(ctx context.Context, cf Confirm) (bool, error) {
	v, err := c.ask(ctx, cf.Page, newConfirm(cf))
	ok, _ := v.(bool)
	return ok, err
}

func (c *client) Form(ctx context.Context, f Form) (Values, error) {
	v, err := c.ask(ctx, f.Page, newForm(f))
	vals, _ := v.(Values)
	return vals, err
}

func (c *client) Notice(ctx context.Context, n Notice) error {
	_, err := c.ask(ctx, n.Page, newNotice(n))
	return err
}

func (c *client) ShowKit(ctx context.Context, k KitScreen) error {
	_, err := c.ask(ctx, k.Page, newKitView(k))
	return err
}

func (c *client) Run(ctx context.Context, r Running, fn func(ctx context.Context, progress func(Progress)) error) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := c.send(ctx, request{page: r.Page, step: newRunning(r, cancel)}); err != nil {
		return err
	}
	return ranTo(ctx, runCtx, fn(runCtx, func(p Progress) { c.post(progressMsg(p)) }))
}

// ranTo is what Run returns for fn's err: ErrCancelled once the program has
// quit, ErrBack when the user cancelled fn, else err itself.
func ranTo(ctx, runCtx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return ErrCancelled
	case runCtx.Err() != nil:
		return ErrBack
	}
	return err
}

// step is what a request shows under its page: it takes the keys, and
// answers once the user has.
type step interface {
	init() tea.Cmd
	update(msg tea.Msg) (*answer, tea.Cmd)
	view(width, height int) stepView
}

// ctrlCStep is a step that handles ctrl+c itself instead of quitting.
type ctrlCStep interface {
	ctrlC() (*answer, tea.Cmd)
}

// errQuit is a step's answer that ends the program.
var errQuit = errors.New("quit")

// stepView is a step's part of the screen.
type stepView struct {
	// content fills the body when the page has none, and otherwise shows
	// in place under the rule.
	content []string
	// notes show under the rule, above the keys: a dashboard's message, a
	// form's error.
	notes []Line
	// notesGiveWay hides notes while there are result lines to show.
	notesGiveWay bool
	keys         string
	help         string
}

// model is the program. Only its own goroutine touches it.
type model struct {
	in     <-chan tea.Msg
	done   <-chan struct{}
	page   Page
	step   step
	reply  chan answer
	banner []bannerLine // result and log lines, until the next key
	logged bool         // the banner has log lines
	dryRun bool
	width  int
	height int
}

func (m *model) listen() tea.Msg {
	select {
	case msg := <-m.in:
		return msg
	case <-m.done:
		return nil
	}
}

func (m *model) Init() tea.Cmd { return m.listen }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case request:
		if msg.page.Title != "" {
			m.page = msg.page
		}
		m.step, m.reply = msg.step, msg.reply
		return m, tea.Batch(m.listen, m.step.init())
	case sayMsg:
		m.banner = append(m.banner, bannerLine{Line: Line(msg)})
		return m, m.listen
	case logMsg:
		if m.dryRun && !m.logged {
			m.banner = append(m.banner, bannerLine{dryRunHeading: true})
		}
		m.logged = true
		m.banner = append(m.banner, bannerLine{Line: Line{Info, string(msg)}, log: true})
		return m, m.listen
	case flowDoneMsg:
		return m, tea.Quit
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case progressMsg:
		if r, ok := m.step.(*running); ok {
			r.setProgress(Progress(msg))
		}
		return m, m.listen
	}
	if m.step == nil {
		return m, nil
	}
	a, cmd := m.step.update(msg)
	return m, tea.Batch(cmd, m.answer(a))
}

// key passes a key to the step. Any key but a running step's clears the
// result lines, which have been seen.
func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	_, busy := m.step.(*running)
	if !busy {
		m.banner, m.logged = nil, false
	}
	ctrlC := k.String() == "ctrl+c"
	switch {
	case m.step == nil || (m.reply == nil && !busy):
		// Nothing asks yet, or the step is answered and the flow is busy
		// until its next request.
		if ctrlC {
			return tea.Quit
		}
		return nil
	case ctrlC:
		if s, ok := m.step.(ctrlCStep); ok {
			a, cmd := s.ctrlC()
			return tea.Batch(cmd, m.answer(a))
		}
		return tea.Quit
	}
	a, cmd := m.step.update(k)
	return tea.Batch(cmd, m.answer(a))
}

// answer replies to the flow, or quits on errQuit.
func (m *model) answer(a *answer) tea.Cmd {
	if a == nil {
		return nil
	}
	if errors.Is(a.err, errQuit) {
		return tea.Quit
	}
	if m.reply != nil {
		m.reply <- *a
		m.reply = nil
	}
	return nil
}

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// width is the frame's: the terminal's, up to frameWidth.
func (m *model) frame() int {
	if m.width <= 0 || m.width > frameWidth {
		return frameWidth
	}
	return m.width
}

// render draws the frame: title and badge, subtitle, body, rule, then the
// result lines, the step's notes and in-place content, its keys and help.
func (m *model) render() string {
	if m.step == nil {
		return ""
	}
	w := m.frame()
	sv := m.step.view(w, m.height)
	page := m.page
	sub := page.Subtitle
	if m.dryRun {
		page.Badge = Badge{Attention, "DRY RUN"}
		dry := styles.Warning.Render("--dry-run: nothing is changed")
		if sub == "" {
			sub = dry
		} else {
			sub += styles.Dimmed.Render(" · ") + dry
		}
	}
	lines := []string{titleLine(page, w), sub, ""}
	var under []string
	if page.Body != "" {
		lines = append(lines, strings.Split(strings.TrimRight(page.Body, "\n"), "\n")...)
	} else {
		lines = append(lines, sv.content...)
	}
	lines = append(lines, "", styles.Dimmed.Render(strings.Repeat("─", w)), "")
	for _, b := range m.banner {
		switch {
		case b.dryRunHeading:
			under = append(under, "  "+styles.Header.Render("Dry run")+"  "+styles.Dimmed.Render("nothing is changed. This is what would run:"))
		case b.log:
			under = append(under, wrap(b.Line, w, 6)...)
		default:
			under = append(under, wrap(b.Line, w, 2)...)
		}
	}
	if !(sv.notesGiveWay && len(m.banner) > 0) {
		for _, n := range sv.notes {
			under = append(under, wrap(n, w, 2)...)
		}
	}
	if page.Body != "" && len(sv.content) > 0 {
		if len(under) > 0 {
			under = append(under, "")
		}
		under = append(under, sv.content...)
	}
	if len(under) > 0 {
		lines = append(lines, under...)
		lines = append(lines, "")
	}
	lines = append(lines, sv.keys)
	if sv.help != "" {
		lines = append(lines, "", "  "+styles.Help.Render(sv.help))
	}
	return strings.Join(lines, "\n")
}

// titleLine is the title, its dim context, and the badge ending at the
// frame's right edge.
func titleLine(p Page, w int) string {
	left := styles.Title.Render(p.Title)
	if p.Context != "" {
		left += styles.Dimmed.Render("  " + p.Context)
	}
	if p.Badge.Text == "" {
		return left
	}
	b := badge(p.Badge)
	pad := w - ansi.StringWidth(left) - ansi.StringWidth(b)
	return left + strings.Repeat(" ", max(pad, 2)) + b
}

func badge(b Badge) string {
	switch b.Tone {
	case Busy:
		return styles.BadgeInfo.Render("● " + b.Text)
	case Attention:
		return styles.BadgeWarn.Render("▲ " + b.Text)
	case Danger:
		return styles.BadgeDanger.Render("✗ " + b.Text)
	}
	return styles.BadgeOK.Render("✓ " + b.Text)
}

// styleLine renders a line of a message, indented two.
func styleLine(l Line) string {
	switch l.Kind {
	case OK:
		return styles.Success.Render(l.Text)
	case Warning:
		return styles.Warning.Render(l.Text)
	case Error:
		return styles.Error.Render(l.Text)
	case Plain:
		return styles.Value.Render(l.Text)
	case Heading:
		return styles.Header.Render(l.Text)
	}
	return styles.Dimmed.Render(l.Text)
}

// bannerLine is a result line, a line of the backup package's output, or
// the heading --dry-run's output goes under.
type bannerLine struct {
	Line
	log           bool
	dryRunHeading bool
}

// wrap styles l, indented two, and wraps it at spaces to the frame with its
// continuation lines indent columns in.
func wrap(l Line, w, indent int) []string {
	var lines []string
	line, limit := "", w-2
	for i, word := range strings.Split(l.Text, " ") {
		if i > 0 && ansi.StringWidth(line)+1+ansi.StringWidth(word) > limit {
			lines = append(lines, line)
			line, limit = word, w-indent
			continue
		}
		if i > 0 {
			line += " "
		}
		line += word
	}
	lines = append(lines, line)
	for i := range lines {
		pre := "  "
		if i > 0 {
			pre = strings.Repeat(" ", indent)
		}
		lines[i] = pre + styleLine(Line{l.Kind, lines[i]})
	}
	return lines
}

// lineWriter hands each line written to it to post: the backup package's
// output, such as --dry-run's, which would tear the screen if it went to the
// terminal.
type lineWriter struct {
	mu   sync.Mutex
	post func(string)
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := strings.TrimRight(string(w.buf[:i]), "\r")
		w.buf = w.buf[i+1:]
		if line != "" {
			w.post(line)
		}
	}
}
