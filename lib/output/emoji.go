package output

var allEmojis = [...]string{
	EmojiFailure,
	EmojiWarning,
	EmojiSuccess,
	EmojiInfo,
	EmojiLightbulb,
	EmojiAsterisk,
	EmojiWarningSign,
	EmojiFingerPointRight,
	EmojiHourglass,
	EmojiShrug,
	EmojiOk,
	EmojiQuestionMark,
}

// Standard emoji for use in output.
const (
	EmojiFailure          = " ⨯"
	EmojiWarning          = "❗️"
	EmojiSuccess          = " ✓"
	EmojiInfo             = "ℹ️"
	EmojiLightbulb        = "💡"
	EmojiAsterisk         = "✱"
	EmojiWarningSign      = "⚠️"
	EmojiFingerPointRight = "👉"
	EmojiHourglass        = "⌛"
	EmojiShrug            = "🤷"
	EmojiOk               = "👌"
	EmojiQuestionMark     = "❔"
)
