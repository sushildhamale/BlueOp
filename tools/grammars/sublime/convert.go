package sublime

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	grammar "github.com/github/linguist/tools/grammars/proto"
)

type Entry struct {
	Match    string         `json:"match"`
	Scope    string         `json:"scope"`
	Captures map[int]string `json:"captures"`
	Push     Context        `json:"push"`
	Pop      bool           `json:"pop"`
	Include  string         `json:"include"`

	MetaContentScope string `json:"meta_content_scope"`
	MetaScope        string `json:"meta_scope"`
}

type Context []Entry

type Syntax struct {
	Name     string             `json:"name"`
	Scope    string             `json:"scope"`
	Contexts map[string]Context `json:"contexts"`
}

type Repository map[string][]*grammar.Rule

func warn(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "WARNING: "+msg+"\n", args...)
}

func formatRegexp(re string) string {
	return re
}

func formatCaptures(from map[string]*grammar.Rule) map[int]string {
	to := make(map[int]string, len(from))
	for k, v := range from {
		if v.Name == "" {
			warn("patterns and includes are not supported within captures")
			continue
		}

		i, err := strconv.ParseInt(k, 10, 32)
		if err != nil {
			warn("named capture used; this is unsupported")
			continue
		}

		to[int(i)] = v.Name
	}
	return to
}

func syntaxForScope(key string) string {
	return "scope:" + key
}

func formatExternalSyntax(key string) string {
	if key[0] == '#' || key[0] == '$' {
		panic("formatting non-external syntax")
	}

	if strings.Contains(key, "#") {
		parts := strings.SplitN(key, "#", 2)
		return syntaxForScope(parts[0]) + "#" + parts[1]
	}

	return syntaxForScope(key)
}

func makeContext(patterns []*grammar.Rule, repo Repository) Context {
	var ctx Context

	for _, p := range patterns {
		if p.Begin != "" {
			var (
				entry         Entry
				endEntry      Entry
				applyLast     bool
				childPatterns []*grammar.Rule
				child         Context
			)

			entry.Match = formatRegexp(p.Begin)
			if len(p.BeginCaptures) != 0 || len(p.Captures) != 0 {
				var captures map[int]string
				if len(p.BeginCaptures) != 0 {
					captures = formatCaptures(p.BeginCaptures)
				} else {
					captures = formatCaptures(p.Captures)
				}

				if rootCapture, ok := captures[0]; ok {
					entry.Scope = rootCapture
					delete(captures, 0)
				}

				if len(captures) != 0 {
					entry.Captures = captures
				}
			}

			endEntry.Match = formatRegexp(p.End)
			endEntry.Pop = true

			if len(p.EndCaptures) != 0 || len(p.Captures) != 0 {
				var captures map[int]string
				if len(p.EndCaptures) != 0 {
					captures = formatCaptures(p.EndCaptures)
				} else {
					captures = formatCaptures(p.Captures)
				}

				if rootCapture, ok := captures[0]; ok {
					endEntry.Scope = rootCapture
					delete(captures, 0)
				}

				if len(captures) != 0 {
					endEntry.Captures = captures
				}
			}

			if strings.Contains(endEntry.Match, "\\G") {
				warn("invalid \\G in regexp: '%s'", endEntry.Match)
			}

			applyLast = p.ApplyEndPatternLast
			if len(p.Patterns) != 0 {
				childPatterns = p.Patterns
			}

			child = makeContext(childPatterns, repo)
			if applyLast {
				child = append(child, endEntry)
			} else {
				child = append(Context{endEntry}, child...)
			}

			if c := p.ContentName; c != "" {
				child = append(Context{Entry{MetaContentScope: c}}, child...)
			}
			if c := p.Name; c != "" {
				child = append(Context{Entry{MetaScope: c}}, child...)
			}

			entry.Push = child
			ctx = append(ctx, entry)
		} else if p.Match != "" {
			var entry Entry
			entry.Match = formatRegexp(p.Match)

			if p.Name != "" {
				entry.Scope = p.Name
			}

			if len(p.Captures) != 0 {
				entry.Captures = formatCaptures(p.Captures)
			}

			ctx = append(ctx, entry)
		} else if p.Include != "" {
			key := p.Include

			if key[0] == '#' {
				key = key[1:]
				if _, ok := repo[key]; !ok {
					warn("no entry in repository for " + key)
				}

				ctx = append(ctx, Entry{Include: key})
			} else if key == "$self" {
				ctx = append(ctx, Entry{Include: "main"})
			} else if key == "$base" {
				ctx = append(ctx, Entry{Include: "$top_level_main"})
			} else if key[0] == '$' {
				warn("unknown include: " + key)
			} else {
				ctx = append(ctx, Entry{Include: formatExternalSyntax(key)})
			}
		} else {
			warn("unknown pattern type")
		}
	}

	return ctx
}

func Convert(rule *grammar.Rule) (*Syntax, error) {
	var repo Repository = make(Repository)
	for key, val := range rule.Repository {
		repo[key] = []*grammar.Rule{val}
	}

	var syn Syntax
	syn.Contexts = map[string]Context{
		"main": makeContext(rule.Patterns, repo),
	}

	for key, val := range repo {
		syn.Contexts[key] = makeContext(val, repo)
	}

	syn.Name = rule.Name
	syn.Scope = rule.ScopeName

	// TODO:
	// comment <- comment
	// file_extensions <- fileTypes
	// first_line_match <- firstLineMatch
	// hidden <- hideFromUser
	// hidden <- hidden

	return &syn, nil
}
