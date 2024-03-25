package assistant

import (
	"context"
	"fmt"
	"github.com/sashabaranov/go-openai"
)

func ConverseWithAI(client *openai.Client, message openai.ChatCompletionMessage, msgHistory []openai.ChatCompletionMessage) openai.ChatCompletionMessage {

	req := openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo,
		Messages: msgHistory,
	}

	req.Messages = append(req.Messages, message)

	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
	}

	return resp.Choices[0].Message
	//return fmt.Sprintf("%s", resp.Choices[0].Message.Content)
	// req.Messages = append(req.Messages, resp.Choices[0].Message)
}
