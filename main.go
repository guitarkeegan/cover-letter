package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	Setup StageMsg = "setup"
	Chat  StageMsg = "chat"
	End   StageMsg = "end"
)

var dbg = func() func(format string, as ...any) {
	if os.Getenv("DEBUG") == "" {
		return func(string, ...any) {}
	}
	file, err := os.Create("log")
	if err != nil {
		log.Fatal("nooooo!!!")
	}
	// truncate = delete the rest
	return func(format string, as ...any) {
		fmt.Fprintf(file, format+"\n", as...)
	}
}()

type StageMsg string

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

type FromAIMsg struct {
	aiConversation []string
	coverLetter    string
}

type AIMsg string
type FileMsg string

type model struct {
	stage        StageMsg
	textarea     textarea.Model
	textInput    textinput.Model
	viewport     viewport.Model
	filepicker   filepicker.Model
	selectedFile string
	toAI         ToAI
	fromAiMsg    FromAIMsg
	userApproved bool
	err          error
}

func sendContextToAI(userInfo ToAI) tea.Cmd {
	// performs io and returns a msg
	return func() tea.Msg {
		return AIMsg("The generated cover letter")
	}
}

// AI will handle it's own state of the conversation
// There is redundancy with storing the conversation here and with the AI
func sendMessageToAI(userMessage string) tea.Cmd {
	return func() tea.Msg {
		return AIMsg("AI response here...")
	}
}

func getLivingDoc(path string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(path)
		if err != nil {
			// panic for now
			log.Fatal("unable to find the file!")
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			log.Fatal("unable to ReadAll")
		}

		text := string(content)

		return FileMsg(text)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	dbg("msg: %[1]T, %[1]v", msg)
	var (
		cmd tea.Cmd
		ti  tea.Cmd
		vp  tea.Cmd
	)

	switch msg := msg.(type) {
	// start with default exit keys
	// send a setup msg 'setup' after event loop
	// in event loop, case to handle msg listening for setup method
	case tea.KeyMsg:
		dbg("  Handling KeyMsg")

		switch msg.Type {
		case tea.KeyCtrlC:
			dbg("    Handling KeyCtrlC")
			return m, tea.Quit

		case tea.KeyEsc:
			dbg("    Handling KeyEsc")
			if m.textarea.Focused() && m.toAI.description == "" {
				m.textarea.Blur()
				m.toAI.description = strings.Trim(m.textarea.Value(), " ")
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.textInput.Focused() {
				dbg("    Handling KeyEnter")
				m.textInput, ti = m.textInput.Update(msg)
				m.viewport, vp = m.viewport.Update(msg)
				m.fromAiMsg.aiConversation = append(m.fromAiMsg.aiConversation, "You: "+m.textInput.Value())
				m.viewport.SetContent(strings.Join(m.fromAiMsg.aiConversation, "\n"))
				m.textInput.Reset()
				m.viewport.GotoBottom()
				return m, tea.Batch(vp, ti, sendMessageToAI(m.textInput.Value()))
			}

		}
	case FileMsg:
		dbg("  Handling FileMsg")
		m.toAI.livingDoc = string(msg)
		m.stage = Chat
		m.textInput.Focus()
		return m, tea.Batch(sendContextToAI(m.toAI), ti)

	case AIMsg:
		dbg("  Handling AIMessage")
		m.viewport, vp = m.viewport.Update(msg)
		m.fromAiMsg.aiConversation = append(m.fromAiMsg.aiConversation, "Assistant: "+string(msg))
		m.viewport.SetContent(strings.Join(m.fromAiMsg.aiConversation, "\n"))
		m.viewport.GotoBottom()
		return m, vp

	}

	dbg("textarea updating")

	m.textarea, cmd = m.textarea.Update(msg)
	if cmd != nil {
		dbg("  returning non nil cmd")
		return m, cmd
	}

	if m.toAI.livingDoc == "" {
		dbg("Update filepicker")
		m.filepicker, cmd = m.filepicker.Update(msg)
		if cmd != nil {
			dbg("  cmd: %v", cmd)
			return m, cmd
		}

		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			dbg("  didSelect!")
			m.selectedFile = path
			m.stage = Chat
			// TODO: read in file
			return m, getLivingDoc(m.selectedFile)
		}
	}

	dbg("Update textInput")
	m.textInput, ti = m.textInput.Update(msg)
	if ti != nil {
		dbg("  ti: %v", ti)
		return m, ti
	}

	return m, nil

}

func (m model) View() string {

	switch m.stage {
	case Setup:
		if m.toAI.description == "" {
			return fmt.Sprintf(
				`Let's make your cover letter!
Paste in the job description below 👇`+"\n\n%s\n\npress 'Escape' when finished", m.textarea.View())
		}
		if m.selectedFile == "" {
			return fmt.Sprintf("\n\nSelect a file where you give your work experience.\n\n%s\n\njob description: %q\n", m.filepicker.View(), m.toAI.description)
		}
	case Chat:
		if !m.userApproved {
			return fmt.Sprintf("%s\n\n%v\n", m.viewport.View(), m.textInput.View())
		}
	case End:
		return "Good luck on the application!"
	}

	return "something else happended..?"

}

func (m model) Init() tea.Cmd {
	initStage := func() tea.Msg {
		dbg("Calling Init")
		return Setup
	}
	fpInit := m.filepicker.Init()

	return tea.Batch(initStage, fpInit)
}

func initialModel() model {

	ta := textarea.New()
	ta.Placeholder = "Paste the job description here..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000
	ta.Focus()

	ti := textinput.New()
	ti.Placeholder = "message to assistant..."
	ti.CharLimit = 200
	ti.Width = 20

	fp := filepicker.New()

	vp := viewport.New(30, 5)
	vp.SetContent(`Press 'Enter' to send a message to the assistant`)

	return model{
		stage:        Setup,
		textarea:     ta,
		textInput:    ti,
		viewport:     vp,
		filepicker:   fp,
		selectedFile: "",
		toAI:         ToAI{},
		fromAiMsg:    FromAIMsg{},
		userApproved: false,
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
