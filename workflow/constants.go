package workflow

// LLMUserMessageTextPointer is the JSON pointer to the first user message
// when there is no system message (input[0].content[0].text).
const LLMUserMessageTextPointer JSONPointer = "/input/0/content/0/text"

// LLMUserMessageTextPointerIndex1 is the JSON pointer to the user message
// when a system message occupies index 0 (input[1].content[0].text).
const LLMUserMessageTextPointerIndex1 JSONPointer = "/input/1/content/0/text"
