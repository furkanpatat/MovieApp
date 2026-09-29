// Package mockchat is a stand-in ChatModel, so the chat UI can be built
// before a real LLM is plugged in. It picks a curated list by keyword.
package mockchat

import (
	"context"
	"strings"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

type Model struct{}

var _ domain.ChatModel = Model{}

type pick struct {
	keywords []string
	reply    string
	replyTR  string // the reply in Turkish (a Turkish UI)
	ids      []int
}

// TMDB ids of well-known titles.
var picks = []pick{
	{
		keywords: []string{"sci", "space", "future", "alien", "robot", "bilim", "uzay", "gelecek", "uzaylı"},
		reply:    "Looking for something mind-bending? These science-fiction picks are a great place to start:",
		replyTR:  "Akıl yakan bir şey mi arıyorsun? Bu bilim kurgu seçkileri harika bir başlangıç:",
		ids:      []int{157336, 27205, 603, 335984, 438631},
	},
	{
		keywords: []string{"funny", "comedy", "laugh", "light", "komik", "komedi", "gül"},
		reply:    "Here are a few comedies to lighten the mood:",
		replyTR:  "Keyfini yerine getirecek birkaç komedi:",
		ids:      []int{120467, 18785, 8363, 546554},
	},
	{
		keywords: []string{"horror", "scary", "creepy", "thriller", "korku", "gerilim"},
		reply:    "Lights off? These should keep you on the edge of your seat:",
		replyTR:  "Işıklar kapalı mı? Bunlar seni koltuğunun ucunda tutacak:",
		ids:      []int{694, 419430, 493922, 346364},
	},
	{
		keywords: []string{"action", "fight", "explosion", "adrenaline", "aksiyon", "dövüş", "patlama"},
		reply:    "Buckle up. Some high-octane action:",
		replyTR:  "Kemerlerini bağla. Adrenalin dolu aksiyon geliyor:",
		ids:      []int{155, 76341, 245891, 361743},
	},
}

var fallback = pick{
	reply: "I'm running in **demo mode** (no `GROQ_API_KEY` set), so here are a few all-time favourites. " +
		"Ask me for a genre or a mood, like \"something funny\" or \"a space adventure\".",
	replyTR: "Şu an **demo modundayım** (`GROQ_API_KEY` ayarlı değil), o yüzden işte tüm zamanların birkaç favorisi. " +
		"Bana bir tür ya da ruh hâli sor, örneğin \"komik bir şey\" ya da \"bir uzay macerası\".",
	ids: []int{27205, 155, 13, 157336, 680},
}

func (Model) Reply(_ context.Context, req domain.ChatRequest) (domain.ChatReply, error) {
	last := strings.ToLower(req.History[len(req.History)-1].Content)
	p := fallback
	for _, c := range picks {
		for _, k := range c.keywords {
			if strings.Contains(last, k) {
				p = c
				break
			}
		}
		if p.reply != fallback.reply {
			break
		}
	}
	msg := p.reply
	if req.Locale == domain.LocaleTR {
		msg = p.replyTR
	}
	return domain.ChatReply{Message: msg, MovieIDs: append([]int(nil), p.ids...)}, nil
}
