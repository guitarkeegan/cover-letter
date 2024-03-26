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
	cl "github.com/guitarkeegan/cover-letter/assistant"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
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

type WithAIMsg struct {
	aiConversation []openai.ChatCompletionMessage
	coverLetter    string
}

type AIChatMsg openai.ChatCompletionMessage
type FileMsg string

type model struct {
	aiClient     *openai.Client
	stage        StageMsg
	textarea     textarea.Model
	textInput    textinput.Model
	viewport     viewport.Model
	filepicker   filepicker.Model
	selectedFile string
	toAI         ToAI
	withAIMsg    WithAIMsg
	userApproved bool
	err          error
}

func (m model) renderViewport() []string {

	var currMessages []string
	for _, cm := range m.withAIMsg.aiConversation {
		switch cm.Role {
		case openai.ChatMessageRoleUser:
			currMessages = append(currMessages, "You: "+cm.Content)
		case openai.ChatMessageRoleAssistant:
			currMessages = append(currMessages, "Assistant: "+cm.Content)
		}
	}
	return currMessages
}

func sendContextToAI(c *openai.Client, userInfo ToAI, msgHistory []openai.ChatCompletionMessage) tea.Cmd {
	// performs io and returns a msg
	compMsg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: fmt.Sprintf("Here is the user's information. Based on the job that they are applying for, and the user's experience, generate the first draft of a cover letter. Make the cover letter concise, and only write about the parts of the user's experience that could be relavent to the job description. No more than 2 paragraphs. Then, ask the user if they would like to make any modifications. Job Description: %s User Experience: %s", userInfo.description, userInfo.livingDoc),
	}
	resp := cl.ConverseWithAI(c, compMsg, msgHistory)

	file, err := os.Create("firstCover")
	if err != nil {
		log.Fatal("Error writing firstCover file!", err)
	}
	file.WriteString(resp.Content)
	err = file.Close()
	if err != nil {
		log.Fatal("Error closing file", err)
	}
	return func() tea.Msg {
		return AIChatMsg(resp)
	}
}

func sendMessageToAI(c *openai.Client, message openai.ChatCompletionMessage, msgHistory []openai.ChatCompletionMessage) tea.Cmd {
	resp := cl.ConverseWithAI(c, message, msgHistory)
	return func() tea.Msg {
		return AIChatMsg(resp)
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

	dbg("Update Viewport")
	dbg("Update textInput")
	m.viewport, vp = m.viewport.Update(msg)
	m.textInput, ti = m.textInput.Update(msg)

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
				//m.viewport, vp = m.viewport.Update(msg)
				chatMessage := openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: m.textInput.Value(),
				}
				m.withAIMsg.aiConversation = append(m.withAIMsg.aiConversation, chatMessage)
				currMessages := m.renderViewport()
				m.viewport.SetContent(strings.Join(currMessages, "\n\n"))
				m.textInput.Reset()
				m.viewport.GotoBottom()
				return m, tea.Batch(vp, ti, sendMessageToAI(m.aiClient, chatMessage, m.withAIMsg.aiConversation))
			}
		case tea.KeyUp:
			dbg("    Handling KeyUp")
			m.viewport.LineUp(1)
		case tea.KeyDown:
			dbg("    Handling KeyDown")
			m.viewport.LineDown(1)
		}
	case FileMsg:
		dbg("  Handling FileMsg")
		m.toAI.livingDoc = string(msg)
		m.stage = Chat
		m.textInput.Focus()
		return m, tea.Batch(sendContextToAI(m.aiClient, m.toAI, m.withAIMsg.aiConversation), ti)

	case AIChatMsg:
		dbg("  Handling AIMessage")
		m.withAIMsg.aiConversation = append(m.withAIMsg.aiConversation, openai.ChatCompletionMessage(msg))
		currMessages := m.renderViewport()
		m.viewport.SetContent(strings.Join(currMessages, "\n\n"))
		m.viewport.GotoBottom()
		return m, vp

	}

	if m.stage == Setup {
		dbg("textarea updating")
		m.textarea, cmd = m.textarea.Update(msg)
		if cmd != nil {
			dbg("  returning non nil cmd")
			return m, cmd

		}
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

	dbg("  end of update!")
	return m, tea.Batch(ti, vp)

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
			return fmt.Sprintf("%s\n%v\n", m.viewport.View(), m.textInput.View())
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

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	apiKey := os.Getenv("OPENAI_API")

	client := openai.NewClient(apiKey)

	aiSetup := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "Your job is to help the user create a taylored cover letter based on the job description, and the user's experience.",
		},
	}

	ta := textarea.New()
	ta.Placeholder = "Paste the job description here..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 3200
	ta.Focus()

	ti := textinput.New()
	ti.Placeholder = "message to assistant..."
	ti.CharLimit = 200
	ti.Width = 80

	fp := filepicker.New()

	vp := viewport.New(80, 30)
	vp.SetContent(`Press 'Enter' to send a message to the assistant`)

	return model{
		aiClient:     client,
		stage:        Setup,
		textarea:     ta,
		textInput:    ti,
		viewport:     vp,
		filepicker:   fp,
		selectedFile: "",
		toAI:         ToAI{},
		withAIMsg: WithAIMsg{
			aiConversation: aiSetup,
			coverLetter:    "",
		},
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
