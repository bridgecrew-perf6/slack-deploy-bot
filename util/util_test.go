package util

import (
	"github.com/joho/godotenv"
	"testing"
)

var users = []struct {
	desc string
	uid  string
	want bool
}{
	{"Authorized user", "U022HC654DP", true},
	{"Another authorized user", "UJ6APF5MF", true},
	{"Unauthorized user", "U8675309", false},
	{"No user provided", "", false},
}

func TestAuthorizeUser(t *testing.T) {
	godotenv.Load("../.env")
	for _, u := range users {
		t.Run(u.desc, func(t *testing.T) {
			got := AuthorizeUser(u.uid)
			if got != u.want {
				t.Errorf("AuthorizeUser(%s) got %v, want %v", u.uid, got, u.want)
			}
		})
	}
}

var apps = []struct {
	desc string
	name string
	want bool
}{
	{"Valid app", "time", true},
	{"Another valid app", "performance", true},
	{"An app with incorrect casing", "Time", false},
	{"Invalid app", "t1me", false},
	{"Another invalid app", "salsa", false},
	{"No app provided", "", false},
	{"Negative one?", "-1", false},
}

func TestCheckAppValid(t *testing.T) {
	godotenv.Load("../.env")
	for _, a := range apps {
		t.Run(a.desc, func(t *testing.T) {
			got := CheckAppValid(a.name)
			if got != a.want {
				t.Errorf("CheckAppValid(%s) got %v, want %v", a.name, got, a.want)
			}
		})
	}
}

var events = []struct {
	desc  string
	event string
	want  bool
}{
	{"Wrong number of args", "time 54", false}, // the app and ref are actually the 2nd and 3rd args in the slackevent
	{"Extraneous whitespace", " time  54", false},
	{"Ref is a natural number", "XXXX time 18", true},
	{"Ref is a non-natural number", "XXXX time -18", false},
	{"Ref is a non-natural number", "XXXX time 0", false},
	{"Ref is not main", "XXXX time maine", false},
	{"Ref is main", "XXXX time main", true},
}

func TestCheckArgsValid(t *testing.T) {
	for _, e := range events {
		t.Run(e.desc, func(t *testing.T) {
			got, _, _, _ := CheckArgsValid(e.event)
			if got != e.want {
				t.Errorf("CheckArgsValid(%s) got %v, want %v", e.event, got, e.want)
			}
		})
	}
}
