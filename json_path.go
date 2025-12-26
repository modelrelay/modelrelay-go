package sdk

import (
	"fmt"
)

// JSONPath provides type-safe JSON pointer construction.
// Use these builders instead of raw strings to get compile-time safety
// and IDE autocomplete for common LLM request/response paths.
//
// Example:
//
//	// Instead of raw string:
//	pointer := "/output/0/content/0/text"
//
//	// Use typed builder:
//	pointer := LLMOutput().Content(0).Text().String()
//
//	// Or use pre-built paths:
//	pointer := LLMOutputText  // same as LLMOutput().Content(0).Text()

// LLMOutputPath builds paths for LLM response output structures.
// Use LLMOutput() to start building a path.
type LLMOutputPath struct {
	path string
}

// LLMOutput starts building a path into an LLM response output.
// The output structure is: output[index].content[index].{text|...}
func LLMOutput() LLMOutputPath {
	return LLMOutputPath{path: "/output"}
}

// Index selects an output by index.
func (p LLMOutputPath) Index(i int) LLMOutputContentPath {
	return LLMOutputContentPath{path: fmt.Sprintf("%s/%d", p.path, i)}
}

// Content is a shorthand for Index(i).Content(j).
func (p LLMOutputPath) Content(i int) LLMOutputContentItemPath {
	return p.Index(0).Content(i)
}

// LLMOutputContentPath represents output[i] level.
type LLMOutputContentPath struct {
	path string
}

// Content selects a content item by index.
func (p LLMOutputContentPath) Content(i int) LLMOutputContentItemPath {
	return LLMOutputContentItemPath{path: fmt.Sprintf("%s/content/%d", p.path, i)}
}

// LLMOutputContentItemPath represents output[i].content[j] level.
type LLMOutputContentItemPath struct {
	path string
}

// Text returns the text field pointer.
func (p LLMOutputContentItemPath) Text() JSONPointer {
	return JSONPointer(p.path + "/text")
}

// Type returns the type field pointer.
func (p LLMOutputContentItemPath) Type() JSONPointer {
	return JSONPointer(p.path + "/type")
}

// String returns the JSON pointer string.
func (p LLMOutputContentItemPath) String() string {
	return p.path
}

// LLMInputPath builds paths for LLM request input structures.
// Use LLMInput() to start building a path.
type LLMInputPath struct {
	path string
}

// LLMInput starts building a path into an LLM request input.
// The input structure is: input[message_index].content[content_index].{text|...}
func LLMInput() LLMInputPath {
	return LLMInputPath{path: "/input"}
}

// Message selects a message by index.
// Index 0 is typically the system message, index 1 is the first user message.
func (p LLMInputPath) Message(i int) LLMInputMessagePath {
	return LLMInputMessagePath{path: fmt.Sprintf("%s/%d", p.path, i)}
}

// SystemMessage is shorthand for Message(0) - the first message slot.
func (p LLMInputPath) SystemMessage() LLMInputMessagePath {
	return p.Message(0)
}

// UserMessage is shorthand for Message(1) - typically the user message after system.
func (p LLMInputPath) UserMessage() LLMInputMessagePath {
	return p.Message(1)
}

// LLMInputMessagePath represents input[i] level.
type LLMInputMessagePath struct {
	path string
}

// Content selects a content item by index.
func (p LLMInputMessagePath) Content(i int) LLMInputContentItemPath {
	return LLMInputContentItemPath{path: fmt.Sprintf("%s/content/%d", p.path, i)}
}

// Text is shorthand for Content(0).Text().
func (p LLMInputMessagePath) Text() JSONPointer {
	return p.Content(0).Text()
}

// LLMInputContentItemPath represents input[i].content[j] level.
type LLMInputContentItemPath struct {
	path string
}

// Text returns the text field pointer.
func (p LLMInputContentItemPath) Text() JSONPointer {
	return JSONPointer(p.path + "/text")
}

// Type returns the type field pointer.
func (p LLMInputContentItemPath) Type() JSONPointer {
	return JSONPointer(p.path + "/type")
}

// String returns the JSON pointer string.
func (p LLMInputContentItemPath) String() string {
	return p.path
}

// Pre-built paths for common operations.
// These provide the same values as the constants but with type-safe construction.
var (
	// LLMOutputText extracts text from the first content item of the first output.
	// Equivalent to: LLMOutput().Content(0).Text()
	LLMOutputText = LLMOutput().Content(0).Text()

	// LLMInputSystemText targets the system message text (input[0].content[0].text).
	// Use when the request has a system message at index 0.
	LLMInputSystemText = LLMInput().SystemMessage().Text()

	// LLMInputUserText targets the user message text (input[1].content[0].text).
	// Use when the request has a system message at index 0 and user at index 1.
	LLMInputUserText = LLMInput().UserMessage().Text()

	// LLMInputFirstMessageText targets the first message text (input[0].content[0].text).
	// Use when there's no system message and user message is at index 0.
	LLMInputFirstMessageText = LLMInput().Message(0).Text()
)
