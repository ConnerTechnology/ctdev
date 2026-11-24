package backupapp

import (
	"context"
	"errors"
	"testing"
)

// fakeUI is a scripted UI for flow tests. Each queue answers its kind of
// call in order; an error in a queue is returned instead of an answer. A call
// whose queue is empty returns ErrCancelled, as if the user had quit, which
// ends the flow. Every request and Say is recorded.
//
//	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
//	err := folderFlow(ctx, ui, deps)
//	ui.said(OK) // the green lines
type fakeUI struct {
	t testing.TB

	Dashboards []any   // the key pressed (string), or an error
	Picks      []any   // the index chosen (int), or an error
	Confirms   []any   // the answer (bool), or an error
	Forms      []any   // the values entered (Values), or an error
	Notices    []error // nil acknowledges
	Kits       []error // nil is "I've saved it"
	// Runs are how each Run ends: nil runs fn to the end; ErrBack runs it
	// with its context already cancelled, as esc does.
	Runs []error

	Requests []any // every Dashboard, Picker, Confirm, Form, Notice, KitScreen and Running, in order
	Said     []Line
	Progress []Progress // every report a Run's fn made
}

func pop[T any](q *[]T) (T, bool) {
	var zero T
	if len(*q) == 0 {
		return zero, false
	}
	v := (*q)[0]
	*q = (*q)[1:]
	return v, true
}

// next answers from q: its value, or its error, or ErrCancelled when empty.
func next[T any](q *[]any) (T, error) {
	var zero T
	v, ok := pop(q)
	if !ok {
		return zero, ErrCancelled
	}
	if err, ok := v.(error); ok {
		return zero, err
	}
	return v.(T), nil
}

func (f *fakeUI) Dashboard(_ context.Context, d Dashboard) (Action, error) {
	f.Requests = append(f.Requests, d)
	key, err := next[string](&f.Dashboards)
	if err != nil {
		return Action{}, err
	}
	for _, a := range d.Actions {
		if a.Key == key && !a.Dimmed {
			return a, nil
		}
	}
	f.t.Errorf("pressed %q on a dashboard whose actions are %+v", key, d.Actions)
	return Action{}, ErrCancelled
}

func (f *fakeUI) Pick(_ context.Context, p Picker) (int, error) {
	f.Requests = append(f.Requests, p)
	i, err := next[int](&f.Picks)
	if err == nil && (i < 0 || i >= len(p.Items) || p.Items[i].Dimmed) {
		f.t.Errorf("picked %d of %+v", i, p.Items)
		return 0, ErrCancelled
	}
	return i, err
}

func (f *fakeUI) Confirm(_ context.Context, c Confirm) (bool, error) {
	f.Requests = append(f.Requests, c)
	return next[bool](&f.Confirms)
}

func (f *fakeUI) Form(_ context.Context, fm Form) (Values, error) {
	f.Requests = append(f.Requests, fm)
	return next[Values](&f.Forms)
}

func (f *fakeUI) Notice(_ context.Context, n Notice) error {
	f.Requests = append(f.Requests, n)
	err, ok := pop(&f.Notices)
	if !ok {
		return ErrCancelled
	}
	return err
}

func (f *fakeUI) ShowKit(_ context.Context, k KitScreen) error {
	f.Requests = append(f.Requests, k)
	err, ok := pop(&f.Kits)
	if !ok {
		return ErrCancelled
	}
	return err
}

func (f *fakeUI) Run(ctx context.Context, r Running, fn func(context.Context, func(Progress)) error) error {
	f.Requests = append(f.Requests, r)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if end, _ := pop(&f.Runs); errors.Is(end, ErrBack) {
		cancel()
	}
	return ranTo(ctx, runCtx, fn(runCtx, func(p Progress) { f.Progress = append(f.Progress, p) }))
}

func (f *fakeUI) Say(kind Kind, text string) { f.Said = append(f.Said, Line{kind, text}) }

// said is the text of every line said of kind.
func (f *fakeUI) said(kind Kind) []string {
	var out []string
	for _, l := range f.Said {
		if l.Kind == kind {
			out = append(out, l.Text)
		}
	}
	return out
}

// requestsOf is every request of type T, in order.
func requestsOf[T any](f *fakeUI) []T {
	var out []T
	for _, r := range f.Requests {
		if v, ok := r.(T); ok {
			out = append(out, v)
		}
	}
	return out
}
