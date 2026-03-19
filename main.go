package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const systemPrompt = `You are a commit message generator. Given a git diff, write a clear and concise conventional commit message.

Rules:
- Use conventional commit format: type(scope): description
- Types: feat, fix, refactor, docs, style, test, chore, perf, ci, build
- Scope is optional, use when obvious
- Description should be lowercase, imperative mood, no period at end
- Keep the first line under 72 characters
- Add a blank line and bullet points for details ONLY if the diff is complex
- Output ONLY the commit message, nothing else`

const maxDiffChars = 8000

func main() {
	provider := flag.String("p", "", "AI provider: openai, claude, gemini, ollama (auto-detected)")
	model := flag.String("m", "", "Model name override")
	yes := flag.Bool("y", false, "Skip confirmation and commit immediately")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: git-ac [options]\n\nAI-powered commit message generator\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	diff := getDiff()
	if len(diff) > maxDiffChars {
		diff = diff[:maxDiffChars] + "\n... (truncated)"
	}

	p := *provider
	if p == "" {
		p = detectProvider()
	}

	fmt.Printf("🤖 Generating commit message with %s...\n", p)

	msg, err := generate(p, *model, diff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%s\n%s\n%s\n\n", strings.Repeat("─", 50), msg, strings.Repeat("─", 50))

	if !*yes {
		fmt.Print("Commit with this message? [Y/n/e(dit)] ")
		var input string
		fmt.Scanln(&input)
		input = strings.ToLower(strings.TrimSpace(input))

		switch input {
		case "", "y", "yes":
			// continue to commit
		case "e", "edit":
			doCommit("-e", "-m", msg)
			return
		default:
			fmt.Println("Aborted.")
			os.Exit(1)
		}
	}

	// Stage all if nothing was staged
	if err := exec.Command("git", "diff", "--cached", "--quiet").Run(); err == nil {
		exec.Command("git", "add", "-A").Run()
	}
	doCommit("-m", msg)
}

func doCommit(args ...string) {
	cmd := exec.Command("git", append([]string{"commit"}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func getDiff() string {
	staged, _ := exec.Command("git", "diff", "--cached").Output()
	if len(bytes.TrimSpace(staged)) > 0 {
		return string(staged)
	}
	unstaged, _ := exec.Command("git", "diff").Output()
	if len(bytes.TrimSpace(unstaged)) > 0 {
		return string(unstaged)
	}
	fmt.Println("No changes detected. Stage files with 'git add' first.")
	os.Exit(1)
	return ""
}

func detectProvider() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "openai"
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		return "gemini"
	}
	return "ollama"
}

func generate(provider, model, diff string) (string, error) {
	userPrompt := fmt.Sprintf("Generate a commit message for this diff:\n\n```diff\n%s\n```", diff)

	switch provider {
	case "openai":
		return callOpenAI(model, userPrompt)
	case "claude":
		return callClaude(model, userPrompt)
	case "gemini":
		return callGemini(model, userPrompt)
	case "ollama":
		return callOllama(model, userPrompt)
	default:
		return "", fmt.Errorf("unknown provider: %s", provider)
	}
}

// --- OpenAI ---

func callOpenAI(model, userPrompt string) (string, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	body := map[string]any{
		"model":       model,
		"temperature": 0.3,
		"max_tokens":  256,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := postJSON("https://api.openai.com/v1/chat/completions", map[string]string{
		"Authorization": "Bearer " + key,
	}, body, &resp); err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// --- Claude ---

func callClaude(model, userPrompt string) (string, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	if model == "" {
		model = "claude-sonnet-4-6-20250514"
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 256,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}

	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := postJSON("https://api.anthropic.com/v1/messages", map[string]string{
		"x-api-key":         key,
		"anthropic-version": "2023-06-01",
	}, body, &resp); err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("no response from Claude")
	}
	return strings.TrimSpace(resp.Content[0].Text), nil
}

// --- Gemini ---

func callGemini(model, userPrompt string) (string, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}

	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": userPrompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":     0.3,
			"maxOutputTokens": 256,
		},
	}

	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, key)
	if err := postJSON(url, nil, body, &resp); err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}
	return strings.TrimSpace(resp.Candidates[0].Content.Parts[0].Text), nil
}

// --- Ollama ---

func callOllama(model, userPrompt string) (string, error) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.2"
	}

	body := map[string]any{
		"model":  model,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.3,
		},
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	var resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := postJSON(host+"/api/chat", nil, body, &resp); err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s — make sure it's running: ollama serve", host)
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

// --- HTTP helper ---

func postJSON(url string, headers map[string]string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return json.Unmarshal(respBody, result)
}
