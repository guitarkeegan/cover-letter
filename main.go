package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	Setup Stage = "setup"
	Chat  Stage = "chat"
	End   Stage = "end"
)

type Stage string

// no deeper cause here
var ErrMissingValue = errors.New("missing value")
var ErrBadUnicode = errors.New("bad unicode")

// classification of error
type UserInputError struct {
	// error that has caused it
	err error
	// name of the input/value
	name string
}

func NewUserInputError(e error, name string) *UserInputError {
	return &UserInputError{
		err:  e,
		name: name,
	}
}

func (e *UserInputError) Error() string {
	return fmt.Sprintf("input %q: %v", e.name, e.err)
}

// cause of the error
func (e *UserInputError) Unwrap() error {
	return e.err
}

// action to take if error encountered
func (e *UserInputError) UserError() string {
	return fmt.Sprintf("I am struggling to handle information that you have provided. The %q input has encountered the error: %v", e.name, e.err)
}

type ToAI struct {
	description string
	livingDoc   string
}

type FromAi struct {
	coverLetter string
}

type model struct {
	stage    Stage
	textarea textarea.Model
	toAI     ToAI
	fromAi   FromAi
	err      error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.textarea.Focused() {
				m.textarea.Blur()
				m.toAI.description = m.textarea.Value()
				return m, nil
			} else {
				return m, tea.Quit
			}
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)

	return m, cmd
}

func (m model) View() string {

	switch m.stage {
	case Setup:
		if m.toAI.description == "" {
			return fmt.Sprintf(
				`Let's make your cover letter!
Paste in the job description below 👇`+"\n\n%s\n\npress esc when finished", m.textarea.View())
		} else {
			return "OK, now for that file..."
		}
	}

	return "something else happended..?"

}

func (m model) Init() tea.Cmd {
	return nil
}

func initialModel() model {

	ta := textarea.New()
	ta.Placeholder = "Paste the job description here..."
	ta.ShowLineNumbers = false
	ta.MaxWidth = 1000
	ta.MaxHeight = 200
	ta.CharLimit = 2000
	ta.Focus()

	return model{
		stage:    Setup,
		toAI:     ToAI{},
		fromAi:   FromAi{},
		textarea: ta,
	}
}

type UserError interface {
	error
	UserError() string
}

func main() {

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
	//	err := NewUserInputError(ErrBadUnicode, "username")
	//	var uiErr *UserInputError
	//var uErr UserError

	// inspect error
	//if errors.As(err, &uErr) {
	//	fmt.Println("hi there", uiErr.UserError())
	//}

	// dev
	//fmt.Println(err)

}
