package skills

import (
	"sort"
	"strings"
	"unicode"
)

func Select(skills []Skill, query string, max int) []Skill {
	if len(skills) == 0 {
		return nil
	}
	if max <= 0 {
		max = DefaultSelectionCap
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}

	type scoredSkill struct {
		skill Skill
		score int
	}

	scored := make([]scoredSkill, 0, len(skills))
	for _, skill := range skills {
		scored = append(scored, scoredSkill{
			skill: skill,
			score: scoreSkill(skill, tokens),
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].skill.Name != scored[j].skill.Name {
			return scored[i].skill.Name < scored[j].skill.Name
		}
		return scored[i].skill.Path < scored[j].skill.Path
	})

	out := make([]Skill, 0, max)
	for _, scoredSkill := range scored {
		if scoredSkill.score <= 0 {
			break
		}
		out = append(out, scoredSkill.skill)
		if len(out) == max {
			break
		}
	}
	return out
}

func skillNames(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skill.Name)
	}
	return out
}

func tokenize(query string) []string {
	return strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func scoreSkill(skill Skill, tokens []string) int {
	if len(tokens) == 0 {
		return 0
	}
	name := strings.ToLower(skill.Name)
	description := strings.ToLower(skill.Description)
	body := strings.ToLower(skill.Body)

	score := 0
	for _, token := range tokens {
		switch {
		case strings.Contains(name, token):
			score += 10
		case strings.Contains(description, token):
			score += 4
		case strings.Contains(body, token):
			score++
		}
	}
	return score
}
