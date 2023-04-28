package scout

type Status string

var (
    WarningSign = "⚠️ " 
	EmojiFingerPointRight = "👉"
)

const Warning Status = "Warning"

type result struct {
	Status  Status
	Message string
}
