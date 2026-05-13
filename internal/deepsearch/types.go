// Package deepsearch contains a small local model for the Sourcegraph
// deepsearch.v1 external API.
package deepsearch

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/api/connect"
)

const (
	createConversationProcedure        = "/deepsearch.v1.Service/CreateConversation"
	getConversationProcedure           = "/deepsearch.v1.Service/GetConversation"
	cancelConversationProcedure        = "/deepsearch.v1.Service/CancelConversation"
	deleteConversationProcedure        = "/deepsearch.v1.Service/DeleteConversation"
	addConversationQuestionProcedure   = "/deepsearch.v1.Service/AddConversationQuestion"
	listConversationSummariesProcedure = "/deepsearch.v1.Service/ListConversationSummaries"
)

// Client calls the Sourcegraph Deep Search external API.
type Client struct {
	connect connect.Client
}

func NewClient(client connect.Client) *Client {
	return &Client{connect: client}
}

func (c *Client) CreateConversation(ctx context.Context, request CreateConversationRequest) (*Conversation, bool, error) {
	var response Conversation
	ok, err := c.connect.NewCall(createConversationProcedure, request).Do(ctx, &response)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &response, true, nil
}

func (c *Client) GetConversation(ctx context.Context, request GetConversationRequest) (*Conversation, bool, error) {
	var response Conversation
	ok, err := c.connect.NewCall(getConversationProcedure, request).Do(ctx, &response)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &response, true, nil
}

func (c *Client) CancelConversation(ctx context.Context, request CancelConversationRequest) (*Conversation, bool, error) {
	var response Conversation
	ok, err := c.connect.NewCall(cancelConversationProcedure, request).Do(ctx, &response)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &response, true, nil
}

func (c *Client) DeleteConversation(ctx context.Context, request DeleteConversationRequest) (bool, error) {
	return c.connect.NewCall(deleteConversationProcedure, request).Do(ctx, nil)
}

func (c *Client) AddConversationQuestion(ctx context.Context, request AddConversationQuestionRequest) (*Question, bool, error) {
	var response Question
	ok, err := c.connect.NewCall(addConversationQuestionProcedure, request).Do(ctx, &response)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &response, true, nil
}

func (c *Client) ListConversationSummaries(ctx context.Context, request ListConversationSummariesRequest) (*ListConversationSummariesResponse, bool, error) {
	var response ListConversationSummariesResponse
	ok, err := c.connect.NewCall(listConversationSummariesProcedure, request).Do(ctx, &response)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &response, true, nil
}

func NewQuestion(text string) Question {
	return Question{
		Input: []InputContentBlock{
			{Question: &QuestionContent{Text: text}},
		},
	}
}

type CreateConversationRequest struct {
	Parent       string       `json:"parent,omitempty"`
	Conversation Conversation `json:"conversation"`
}

type GetConversationRequest struct {
	Name string `json:"name"`
}

type CancelConversationRequest struct {
	Name string `json:"name"`
}

type DeleteConversationRequest struct {
	Name string `json:"name"`
}

type AddConversationQuestionRequest struct {
	Parent   string   `json:"parent"`
	Question Question `json:"question"`
}

type ListConversationSummariesRequest struct {
	Parent    string                            `json:"parent,omitempty"`
	PageSize  int                               `json:"pageSize,omitempty"`
	PageToken string                            `json:"pageToken,omitempty"`
	Filters   []ListConversationSummariesFilter `json:"filters,omitempty"`
}

type ListConversationSummariesFilter struct {
	ContentQuery string `json:"contentQuery,omitempty"`
	Starred      *bool  `json:"starred,omitempty"`
}

type ListConversationSummariesResponse struct {
	ConversationSummaries []ConversationSummary `json:"conversationSummaries,omitempty"`
	NextPageToken         string                `json:"nextPageToken,omitempty"`
}

type Conversation struct {
	Name       string             `json:"name,omitempty"`
	State      *ConversationState `json:"state,omitempty"`
	CreateTime string             `json:"createTime,omitempty"`
	UpdateTime string             `json:"updateTime,omitempty"`
	Title      string             `json:"title,omitempty"`
	Questions  []Question         `json:"questions,omitempty"`
	URL        string             `json:"url,omitempty"`
}

type ConversationSummary struct {
	Name       string `json:"name,omitempty"`
	Title      string `json:"title,omitempty"`
	CreateTime string `json:"createTime,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
	URL        string `json:"url,omitempty"`
}

type ConversationState struct {
	Processing *StateProcessing `json:"processing,omitempty"`
	Completed  *StateCompleted  `json:"completed,omitempty"`
	Error      *StateError      `json:"error,omitempty"`
	Canceled   *StateCanceled   `json:"canceled,omitempty"`
}

type StateProcessing struct{}

type StateCompleted struct{}

type StateCanceled struct{}

type StateError struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	RetryTime string `json:"retryTime,omitempty"`
}

type Question struct {
	Input      []InputContentBlock  `json:"input,omitempty"`
	Answer     []AnswerContentBlock `json:"answer,omitempty"`
	CreateTime string               `json:"createTime,omitempty"`
}

type InputContentBlock struct {
	Question *QuestionContent `json:"question,omitempty"`
	Image    *ImageContent    `json:"image,omitempty"`
}

type QuestionContent struct {
	Text string `json:"text"`
}

type ImageContent struct {
	URI string `json:"uri"`
}

type AnswerContentBlock struct {
	Markdown *MarkdownContent `json:"markdown,omitempty"`
}

type MarkdownContent struct {
	Text string `json:"text"`
}
