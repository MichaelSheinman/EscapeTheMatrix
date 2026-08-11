package api

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
)

/*
The phone (§3.14, §3.15): every person anything has passed with, and what.

# Why this is a surface at all

The daily conflict was, until this file, three things on one screen: the moment
in front of you, a roster of the people you live among, and a fortnight of
counts. The roster is gone, because a panel that said "Petra agrees with you
warmly in public, then does the other thing" handed the player the conclusion
they are supposed to reach by living beside her (§3.2). What is left is a
question that had nowhere to go: *then how does a player ever learn who these
people are?*

The answer this surface gives is the only one the design allows. They read what
happened. A thread is every dealing with one person in the order it happened —
what they did, what your character did about it, and what came of that — and
everything a player concludes about somebody is concluded off that. Six messages
from Petra is an argument. One sentence describing Petra is a spoiler.

# What is not on it

**No summary of anybody.** [conflict.Thread] carries a name and the messages.
The authored `does` exists, internal/environment loads it, and it does not cross
this wire.

**No unread count, and no badge.** At most one message in the whole phone is
ever waiting, because nothing new arises until the last one is dealt with — so a
count would only ever be counting to one, and a badge saying `1` would be a
notification area, which the shell deliberately does not have.

**No composing.** The replies are the authored answers to the one waiting
moment, sent with the message they answer. There is no text field, because a
player writing their own words would be a different game and a model reading
them would be §7's line crossed.

# Why it is fetched rather than hydrated

A thread grows for as long as somebody lives somewhere, and a world model is
bounded (§10.2). Same argument as the boards and the event record, same shape:
a GET that writes nothing, thrown away with the screen that opened it.
*/

// phonePath is every thread this player has, newest thread first.
const phonePath = "/api/phone"

/*
messageView is one dealing, as a thread reads it back.

Three fields carry the whole exchange and they are all the writing being
reported: `says` is what they did, `did` is what your character did about it, and
`came` is what followed. A client renders them; nothing composes them, for the
same reason internal/playbook renders a rule server-side (§10.2).

`covered` is the field the phone exists for. A message your rules answered is one
your character replied to while you were somewhere else, and finding out what
they said in your name is §3.14's step 1 — the delegation loop's whole payoff,
which used to be a table of tallies and is now the conversation itself.
*/
type messageView struct {
	ID   int64  `json:"id"`
	At   string `json:"at"`
	On   string `json:"on"`
	Name string `json:"name"`
	Says string `json:"says"`
	// Did and Came are empty exactly while the message is waiting.
	Did  string `json:"did"`
	Came string `json:"came"`
	// Covered is whether the playbook answered without asking.
	Covered bool `json:"covered"`
	// Outcome is good | mixed | poor, and pending while it waits. Sent because
	// what came of something is a fact, and never rendered as a grade: §3.2
	// labels no choice, and a thread that coloured your own replies would be
	// doing exactly that.
	Outcome string `json:"outcome"`
	Waiting bool   `json:"waiting"`
	// Answers is what may be said back, and is empty for everything that is not
	// waiting. Carried on the message rather than beside the thread so that a
	// reply is always attached to the thing it replies to.
	Answers []answerView `json:"answers"`
}

/*
threadView is one person and everything that has passed between you.

`here` is whether they are still somebody you live among. A player who moved off
the east side keeps the thread and cannot receive anything new in it, which is
true and is worth showing: the conversation going quiet is what leaving somewhere
actually looks like (§3.15).
*/
type threadView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Where they turn up, by city key, so a client can say where to find them
	// without holding its own map.
	Where string `json:"where"`
	// Environment is the population they belong to, by key. Two populations may
	// key somebody the same way and they are not the same somebody.
	Environment string `json:"environment"`
	Here        bool   `json:"here"`
	// Messages is oldest first, which is the order a conversation is read in.
	Messages []messageView `json:"messages"`
	Waiting  bool          `json:"waiting"`
	// Last is when anything last passed between you.
	Last string `json:"last"`
}

type phoneView struct {
	Threads []threadView `json:"threads"`
}

func threadsOf(threads []conflict.Thread) phoneView {
	out := phoneView{Threads: make([]threadView, 0, len(threads))}
	for _, t := range threads {
		row := threadView{
			Key:         t.Person.Key,
			Name:        t.Person.Name,
			Where:       t.Person.Found,
			Environment: t.Environment,
			Here:        t.Here,
			Messages:    make([]messageView, 0, len(t.Messages)),
			Waiting:     t.Waiting,
			Last:        stamp(t.Last),
		}
		for _, m := range t.Messages {
			msg := messageView{
				ID:      m.ID,
				At:      stamp(m.At),
				On:      m.On.At().Format("2006-01-02"),
				Name:    m.Name,
				Says:    m.Says,
				Did:     m.Did,
				Came:    m.Came,
				Covered: m.Covered,
				Outcome: string(m.Outcome),
				Waiting: m.Waiting,
				Answers: []answerView{},
			}
			for _, a := range m.Answers {
				msg.Answers = append(msg.Answers, answerView{Key: string(a.Stance), Does: a.Did})
			}
			row.Messages = append(row.Messages, msg)
		}
		out.Threads = append(out.Threads, row)
	}
	return out
}

/*
Every thread, on opening the phone.

Unpaged, and bounded by [conflict.Threaded] rather than by a page size. A page
would be the wrong shape twice: threads are bounded by the population — six
people, not six hundred — and a conversation read backwards a page at a time is
not a conversation. What bounds this is how far back it reads, which is a
statement about how much of a life is worth carrying rather than about how much
fits on a screen.
*/
func (s *Server) phone(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("your messages could not be read"))
		return
	}

	var threads []conflict.Thread
	if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		threads, err = s.conflicts.Threads(r.Context(), tx, actor.ID)
		return err
	}); err != nil {
		s.log.Error("read threads", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("your messages could not be read"))
		return
	}

	writeJSON(w, http.StatusOK, threadsOf(threads))
}
