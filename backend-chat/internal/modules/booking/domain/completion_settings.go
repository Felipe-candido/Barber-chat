package domain

type CompletionMode string

const (
	CompletionAutomatic CompletionMode = "automatic"
	CompletionManual    CompletionMode = "manual"
)

func (m CompletionMode) Valid() bool { return m == CompletionAutomatic || m == CompletionManual }
