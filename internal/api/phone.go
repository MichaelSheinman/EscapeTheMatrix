package api

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
)

/*
The phone (§3.14, §3.15): every person anything has passed with, and what.

A player learns who these people are by reading what happened. A thread is every
dealing with one person in the order it happened: what they did, what your
character did about it, and what came of that. Six messages from Petra are the
evidence; one sentence describing Petra hands over the conclusion.

So there is no summary of anybody: [conflict.Thread] carries a name and the
messages, and the authored does never crosses this wire. There is no unread count
or badge, because at most one message in the phone is ever waiting. And there is
no composing — the replies are the authored answers to the waiting moment, sent
with the message they answer (§7).

Fetched rather than hydrated: a thread grows for as long as somebody lives
somewhere, and a world model is bounded (§10.2).
*/

// phonePath is every thread this player has, newest thread first.
const phonePath = "/api/phone"

// messageView is one dealing, as a thread reads it back. says is what they did,
// did is what your character did about it, and came is what followed — all three
// the writing being reported, rendered by the client and composed by nobody
// (§10.2).
//
// covered is the field the phone exists for: a message your rules answered is
// one your character replied to while you were somewhere else, and finding out
// what they said in your name is §3.14's step 1.
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
	// Outcome is good | mixed | poor, and pending while it waits. A fact rather
	// than a grade — a thread that coloured your own replies would be labelling
	// a choice (§3.2).
	Outcome string `json:"outcome"`
	Waiting bool   `json:"waiting"`
	// Answers is what may be said back, empty for everything not waiting.
	// Carried on the message so a reply is attached to what it replies to.
	Answers []answerView `json:"answers"`
}

// threadView is one person and everything that has passed between you. here is
// whether they are still somebody you live among: a player who moved out of
// Wilmington keeps the thread and receives nothing new in it, and the
// conversation going quiet is what leaving somewhere looks like (§3.15).
type threadView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Where they turn up, by city key, so a client can say where to find them
	// without holding its own map.
	Where string `json:"where"`
	// Environment is the population they belong to, by key. Two populations may
	// key somebody the same way and they are not the same somebody.
	Environment string `json:"environment"`
	// Among is that same population as a person reads it, so a quiet thread can
	// name the place it went quiet with.
	Among string `json:"among"`
	Here  bool   `json:"here"`
	// Intro is how the two of them met, ahead of every message and the same for
	// every player. them says who is speaking; false is the player.
	Intro []turnView `json:"intro"`
	// Messages is oldest first, which is the order a conversation is read in.
	Messages []messageView `json:"messages"`
	Waiting  bool          `json:"waiting"`
	// Last is when anything last passed between you.
	Last string `json:"last"`
}

// turnView is one message in an introduction. It carries no id, day or outcome
// because none of those are things an introduction has.
type turnView struct {
	Them bool   `json:"them"`
	Says string `json:"says"`
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
			Among:       t.Among,
			Here:        t.Here,
			Intro:       make([]turnView, 0, len(t.Intro)),
			Messages:    make([]messageView, 0, len(t.Messages)),
			Waiting:     t.Waiting,
			Last:        stamp(t.Last),
		}
		for _, turn := range t.Intro {
			row.Intro = append(row.Intro, turnView{Them: turn.Them, Says: turn.Says})
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

// phone is every thread, on opening it. Unpaged, and bounded by
// [conflict.Threaded] rather than by a page size: threads are bounded by the
// population — six people, not six hundred — and a conversation read backwards a
// page at a time is not a conversation.
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
