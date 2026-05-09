package util

import "github.com/bwmarrin/discordgo"

// EmojiForAPI converts the stored emoji format to the format Discord API expects for reactions.
// <:name:id>  → name:id
// <a:name:id> → a:name:id
// 🎮          → 🎮 (unchanged)
func EmojiForAPI(emoji string) string {
	if len(emoji) > 2 && emoji[0] == '<' && emoji[len(emoji)-1] == '>' {
		return emoji[1 : len(emoji)-1] // strip < and >
	}
	return emoji
}

// EmojiFromReaction reconstructs the stored emoji format from a Discord reaction event.
// custom emoji  → <:name:id> or <a:name:id>
// unicode emoji → 🎮
func EmojiFromReaction(e discordgo.Emoji) string {
	if e.ID == "" {
		return e.Name
	}
	if e.Animated {
		return "<a:" + e.Name + ":" + e.ID + ">"
	}
	return "<:" + e.Name + ":" + e.ID + ">"
}
