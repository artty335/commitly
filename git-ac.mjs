#!/usr/bin/env node
/**
 * git-ac: AI-powered commit message generator (Node.js version)
 * Zero dependencies — uses only built-in Node.js modules
 */

import { execSync } from "child_process";
import { request } from "https";
import { request as httpRequest } from "http";
import { createInterface } from "readline";

const SYSTEM_PROMPT = `You are a commit message generator. Given a git diff, write a clear and concise conventional commit message.

Rules:
- Use conventional commit format: type(scope): description
- Types: feat, fix, refactor, docs, style, test, chore, perf, ci, build
- Scope is optional, use when obvious
- Description should be lowercase, imperative mood, no period at end
- Keep the first line under 72 characters
- Add a blank line and bullet points for details ONLY if the diff is complex
- Output ONLY the commit message, nothing else`;

const MAX_DIFF = 8000;

// --- Main ---

const args = process.argv.slice(2);
const flags = {};
for (let i = 0; i < args.length; i++) {
  if (args[i] === "-p" || args[i] === "--provider") flags.provider = args[++i];
  else if (args[i] === "-m" || args[i] === "--model") flags.model = args[++i];
  else if (args[i] === "-y" || args[i] === "--yes") flags.yes = true;
  else if (args[i] === "-h" || args[i] === "--help") {
    console.log(`Usage: git-ac [options]

AI-powered commit message generator

Options:
  -p, --provider  AI provider: openai, claude, gemini, ollama (auto-detected)
  -m, --model     Model name override
  -y, --yes       Skip confirmation and commit immediately
  -h, --help      Show this help`);
    process.exit(0);
  }
}

let diff = run("git diff --cached").trim();
if (!diff) diff = run("git diff").trim();
if (!diff) { console.log("No changes detected. Stage files with 'git add' first."); process.exit(1); }
if (diff.length > MAX_DIFF) diff = diff.slice(0, MAX_DIFF) + "\n... (truncated)";

const provider = flags.provider || detectProvider();
console.log(`🤖 Generating commit message with ${provider}...`);

const userPrompt = `Generate a commit message for this diff:\n\n\`\`\`diff\n${diff}\n\`\`\``;

try {
  const msg = await generate(provider, flags.model, userPrompt);
  console.log(`\n${"─".repeat(50)}\n${msg}\n${"─".repeat(50)}\n`);

  if (flags.yes) {
    commitChanges(msg);
  } else {
    const answer = await ask("Commit with this message? [Y/n/e(dit)] ");
    const a = answer.trim().toLowerCase();
    if (a === "" || a === "y" || a === "yes") {
      commitChanges(msg);
    } else if (a === "e" || a === "edit") {
      commitChanges(msg, true);
    } else {
      console.log("Aborted.");
      process.exit(1);
    }
  }
} catch (e) {
  console.error(`Error: ${e.message}`);
  process.exit(1);
}

// --- Helpers ---

function run(cmd) {
  try { return execSync(cmd, { encoding: "utf8", maxBuffer: 10 * 1024 * 1024 }); }
  catch { return ""; }
}

function commitChanges(msg, edit = false) {
  const staged = run("git diff --cached --quiet");
  if (staged === "") execSync("git add -A", { stdio: "inherit" });
  const editFlag = edit ? " -e" : "";
  execSync(`git commit${editFlag} -m ${JSON.stringify(msg)}`, { stdio: "inherit" });
}

function ask(q) {
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  return new Promise((r) => rl.question(q, (a) => { rl.close(); r(a); }));
}

function detectProvider() {
  if (process.env.ANTHROPIC_API_KEY) return "claude";
  if (process.env.OPENAI_API_KEY) return "openai";
  if (process.env.GEMINI_API_KEY) return "gemini";
  return "ollama";
}

async function generate(provider, model, userPrompt) {
  switch (provider) {
    case "openai": return callOpenAI(model, userPrompt);
    case "claude": return callClaude(model, userPrompt);
    case "gemini": return callGemini(model, userPrompt);
    case "ollama": return callOllama(model, userPrompt);
    default: throw new Error(`Unknown provider: ${provider}`);
  }
}

// --- Providers ---

async function callOpenAI(model, userPrompt) {
  const key = process.env.OPENAI_API_KEY;
  if (!key) throw new Error("OPENAI_API_KEY not set");
  const data = await post("https://api.openai.com/v1/chat/completions", {
    "Authorization": `Bearer ${key}`,
  }, {
    model: model || "gpt-4o-mini",
    temperature: 0.3,
    max_tokens: 256,
    messages: [
      { role: "system", content: SYSTEM_PROMPT },
      { role: "user", content: userPrompt },
    ],
  });
  return data.choices?.[0]?.message?.content?.trim() || throwErr("No response from OpenAI");
}

async function callClaude(model, userPrompt) {
  const key = process.env.ANTHROPIC_API_KEY;
  if (!key) throw new Error("ANTHROPIC_API_KEY not set");
  const data = await post("https://api.anthropic.com/v1/messages", {
    "x-api-key": key,
    "anthropic-version": "2023-06-01",
  }, {
    model: model || "claude-sonnet-4-6-20250514",
    max_tokens: 256,
    system: SYSTEM_PROMPT,
    messages: [{ role: "user", content: userPrompt }],
  });
  return data.content?.[0]?.text?.trim() || throwErr("No response from Claude");
}

async function callGemini(model, userPrompt) {
  const key = process.env.GEMINI_API_KEY;
  if (!key) throw new Error("GEMINI_API_KEY not set");
  const m = model || "gemini-2.0-flash";
  const url = `https://generativelanguage.googleapis.com/v1beta/models/${m}:generateContent?key=${key}`;
  const data = await post(url, {}, {
    system_instruction: { parts: [{ text: SYSTEM_PROMPT }] },
    contents: [{ parts: [{ text: userPrompt }] }],
    generationConfig: { temperature: 0.3, maxOutputTokens: 256 },
  });
  return data.candidates?.[0]?.content?.parts?.[0]?.text?.trim() || throwErr("No response from Gemini");
}

async function callOllama(model, userPrompt) {
  const host = process.env.OLLAMA_HOST || "http://localhost:11434";
  try {
    const data = await post(`${host}/api/chat`, {}, {
      model: model || "llama3.2",
      stream: false,
      options: { temperature: 0.3 },
      messages: [
        { role: "system", content: SYSTEM_PROMPT },
        { role: "user", content: userPrompt },
      ],
    });
    return data.message?.content?.trim() || throwErr("No response from Ollama");
  } catch {
    throw new Error(`Cannot connect to Ollama at ${host} — make sure it's running: ollama serve`);
  }
}

function throwErr(msg) { throw new Error(msg); }

// --- HTTP helper ---

function post(url, headers, body) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const fn = u.protocol === "https:" ? request : httpRequest;
    const req = fn(u, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...headers },
    }, (res) => {
      let data = "";
      res.on("data", (c) => (data += c));
      res.on("end", () => {
        if (res.statusCode >= 400) reject(new Error(`API error ${res.statusCode}: ${data}`));
        else resolve(JSON.parse(data));
      });
    });
    req.on("error", reject);
    req.end(JSON.stringify(body));
  });
}
