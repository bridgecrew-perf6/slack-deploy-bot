package util_test

import (
	"deploy-bot/util"
	"github.com/joho/godotenv"
	"testing"
)

func TestAuthorizeUser(t *testing.T) {
	godotenv.Load("../.env")
	type user struct {
		desc string
		uid  string
		want bool
	}
	tt := []user{
		{"Authorized user", "U022HC654DP", true},
		{"Another authorized user", "UJ6APF5MF", true},
		{"Unauthorized user", "U8675309", false},
		{"No user provided", "", true},
	}

	for i, u := range tt {
		t.Run(u.desc, func(t *testing.T) {
			//	t.Parallel()
			got := util.AuthorizeUser(u.uid)
			if got != u.want {
				t.Errorf("Test %d: AuthorizeUser(%s) got %v, want %v", i+1, u.uid, got, u.want)
			}
		})
	}
}

func TestCheckAppValid(t *testing.T) {
	godotenv.Load("../.env")
	type app struct {
		desc string
		name string
		want bool
	}
	tt := []app{
		{"Valid app", "time", true},
		{"Another valid app", "performance", true},
		{"Invalid casing", "Time", false},
		{"Invalid app", "t1me", false},
		{"Another invalid app", "salsa", false},
		{"No app provided", "", false},
		{"Negative one???", "-1", false},
	}

	for i, a := range tt {
		t.Run(a.desc, func(t *testing.T) {
			got := util.CheckAppValid(a.name)
			if got != a.want {
				t.Errorf("Test %d: CheckAppValid(%s) got %v, want %v", i+1, a.name, got, a.want)
			}
		})
	}
}

func TestCheckArgsValid(t *testing.T) {
	tt := []struct {
		desc  string
		event string
		want  bool
	}{
		{"Wrong number of args", "time 54", false}, // app and ref are actually the 2nd and 3rd args in the slackevent
		{"Extraneous whitespace", " time  54", false},
		{"Ref is a natural number", "XXXX time 18", true},
		{"Ref is a non-natural number", "XXXX time -18", false},
		{"Ref is a non-natural number", "XXXX time 0", false},
		{"Ref is not main", "XXXX time maine", false},
		{"Ref is main", "XXXX time main", true},
	}
	for i, e := range tt {
		t.Run(e.desc, func(t *testing.T) {
			got, _, _, _ := util.CheckArgsValid(e.event)
			if got != e.want {
				t.Errorf("Test %d: CheckArgsValid(%s) got %v, want %v", i+1, e.event, got, e.want)
			}
		})
	}
}

func TestBuildDockerImageString(t *testing.T) {
	type imageString struct {
		desc string
		ref  string
		sha  string
		want string
	}
	tt := []imageString{
		{"Main branch + sha", "main", "deadbeefdeadbeef", "main-deadbee"},
		{"Feat branch + sha", "feat", "abcdef1234567890", "feat-abcdef1"},
		{"Another Feat branch + sha", "neat-feat", "1234567890abcdef", "neat-feat-1234567"},
		{"Another Feat branch + sha", "feat", "abcdef1234567890", "feat-abcdef1"},
	}

	for i, s := range tt {
		t.Run(s.desc, func(t *testing.T) {
			got := *util.BuildDockerImageString(s.ref, s.sha)
			if got != s.want {
				t.Errorf("Test %d: BuildDockerImageString(%s,%s) got %v, want %v", i+1, s.ref, s.sha, got, s.want)
			}
		})
	}
}
