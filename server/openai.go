package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmorganca/ollama/api"
)

type OpenAIError struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Param   interface{} `json:"param"`
	Code    *string     `json:"code"`
}

type OpenAIErrorResponse struct {
	Error OpenAIError `json:"error"`
}

type OpenAIChatCompletionRequest struct {
	Model    string
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (m *OpenAIMessage) toMessage() api.Message {
	return api.Message{
		Role:    m.Role,
		Content: m.Content,
	}
}

// non-streaming response

type OpenAIChatCompletionResponseChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason *string       `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionResponse struct {
	ID                string                               `json:"id"`
	Object            string                               `json:"object"`
	Created           int64                                `json:"created"`
	Model             string                               `json:"model"`
	SystemFingerprint string                               `json:"system_fingerprint"`
	Choices           []OpenAIChatCompletionResponseChoice `json:"choices"`
	Usage             OpenAIUsage                          `json:"usage,omitempty"`
}

// streaming response

type OpenAIChatCompletionResponseChoiceStream struct {
	Index        int           `json:"index"`
	Delta        OpenAIMessage `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

type OpenAIChatCompletionResponseStream struct {
	ID                string                                     `json:"id"`
	Object            string                                     `json:"object"`
	Created           int64                                      `json:"created"`
	Model             string                                     `json:"model"`
	SystemFingerprint string                                     `json:"system_fingerprint"`
	Choices           []OpenAIChatCompletionResponseChoiceStream `json:"choices"`
}

type StreamCompletionMarker struct{} // signals to send [DONE] on the event-stream

func ChatCompletions(c *gin.Context) {
	var req OpenAIChatCompletionRequest
	err := c.ShouldBindJSON(&req)
	switch {
	case errors.Is(err, io.EOF):
		c.AbortWithStatusJSON(http.StatusBadRequest, OpenAIErrorResponse{
			OpenAIError{
				Message: "missing request body",
				Type:    "invalid_request_error",
			},
		})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusBadRequest, OpenAIErrorResponse{
			OpenAIError{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Call generate and receive the channel with the responses
	chatReq := api.ChatRequest{
		Model:  req.Model,
		Stream: &req.Stream,
	}
	for _, m := range req.Messages {
		chatReq.Messages = append(chatReq.Messages, m.toMessage())
	}
	ch, err := chat(c, chatReq, time.Now())
	if err != nil {
		var pErr *fs.PathError
		switch {
		case errors.As(err, &pErr):
			c.AbortWithStatusJSON(http.StatusBadRequest, OpenAIErrorResponse{
				OpenAIError{
					Message: fmt.Sprintf("model '%s' not found, try pulling it first", req.Model),
					Type:    "invalid_request_error",
				},
			})
		case errors.Is(err, api.ErrInvalidOpts), errors.Is(err, errInvalidRole):
			c.AbortWithStatusJSON(http.StatusBadRequest, OpenAIErrorResponse{
				OpenAIError{
					Message: err.Error(),
					Type:    "invalid_request_error",
				},
			})
		default:
			c.AbortWithStatusJSON(http.StatusInternalServerError, OpenAIErrorResponse{
				OpenAIError{
					Message: err.Error(),
					Type:    "internal_server_error",
				},
			})
		}
		return
	}

	if !req.Stream {
		// Wait for the channel to close
		var chatResponse api.ChatResponse
		var sb strings.Builder

		for val := range ch {
			var ok bool
			chatResponse, ok = val.(api.ChatResponse)
			if !ok {
				c.AbortWithStatusJSON(http.StatusBadRequest, OpenAIErrorResponse{
					OpenAIError{
						Message: err.Error(),
						Type:    "internal_server_error",
					},
				})
				return
			}
			if chatResponse.Message != nil {
				sb.WriteString(chatResponse.Message.Content)
			}

			if chatResponse.Done {
				chatResponse.Message = &api.Message{Role: "assistant", Content: sb.String()}
			}
		}
		// Send a single response with accumulated content
		id := fmt.Sprintf("chatcmpl-%d", rand.Intn(999))
		chatCompletionResponse := OpenAIChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion",
			Created: chatResponse.CreatedAt.Unix(),
			Model:   req.Model,
			Choices: []OpenAIChatCompletionResponseChoice{
				{
					Index: 0,
					Message: OpenAIMessage{
						Role:    "assistant",
						Content: chatResponse.Message.Content,
					},
					FinishReason: func(done bool) *string {
						if done {
							reason := "stop"
							return &reason
						}
						return nil
					}(chatResponse.Done),
				},
			},
		}
		c.JSON(http.StatusOK, chatCompletionResponse)
		return
	}

	// Now, create the intermediate channel and transformation goroutine
	transformedCh := make(chan any)

	go func() {
		defer close(transformedCh)
		id := fmt.Sprintf("chatcmpl-%d", rand.Intn(999)) // TODO: validate that this does not change with each chunk
		predefinedResponse := OpenAIChatCompletionResponseStream{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []OpenAIChatCompletionResponseChoiceStream{
				{
					Index: 0,
					Delta: OpenAIMessage{
						Role: "assistant",
					},
				},
			},
		}
		transformedCh <- predefinedResponse
		for val := range ch {
			resp, ok := val.(api.ChatResponse)
			if !ok {
				// If val is not of type ChatResponse, send an error down the channel and exit
				transformedCh <- OpenAIErrorResponse{
					OpenAIError{
						Message: "failed to parse chat response",
						Type:    "internal_server_error",
					},
				}
				return
			}

			// Transform the ChatResponse into OpenAIChatCompletionResponse
			chatCompletionResponse := OpenAIChatCompletionResponseStream{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: resp.CreatedAt.Unix(),
				Model:   resp.Model,
				Choices: []OpenAIChatCompletionResponseChoiceStream{
					{
						Index: 0,
						FinishReason: func(done bool) *string {
							if done {
								reason := "stop"
								return &reason
							}
							return nil
						}(resp.Done),
					},
				},
			}
			if resp.Message != nil {
				chatCompletionResponse.Choices[0].Delta = OpenAIMessage{
					Content: resp.Message.Content,
				}
			}
			transformedCh <- chatCompletionResponse
			if resp.Done {
				transformedCh <- StreamCompletionMarker{}
			}
		}
	}()

	// Pass the transformed channel to streamResponse
	streamOpenAIResponse(c, transformedCh)
}

func streamOpenAIResponse(c *gin.Context, ch chan any) {
	c.Header("Content-Type", "text/event-stream")
	c.Stream(func(w io.Writer) bool {
		val, ok := <-ch
		if !ok {
			return false
		}

		// Check if the message is a StreamCompletionMarker to close the event stream
		if _, isCompletionMarker := val.(StreamCompletionMarker); isCompletionMarker {
			if _, err := w.Write([]byte("data: [DONE]\n")); err != nil {
				log.Printf("streamOpenAIResponse: w.Write failed with %s", err)
				return false
			}
			return false // Stop streaming after sending [DONE]
		}

		bts, err := json.Marshal(val)
		if err != nil {
			log.Printf("streamOpenAIResponse: json.Marshal failed with %s", err)
			return false
		}

		formattedResponse := fmt.Sprintf("data: %s\n", bts)

		if _, err := w.Write([]byte(formattedResponse)); err != nil {
			log.Printf("streamOpenAIResponse: w.Write failed with %s", err)
			return false
		}

		return true
	})
}
