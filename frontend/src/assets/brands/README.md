# Brand logos

Official provider logos used in the AI model list, the model form and the chat
model picker.

- Source package: `@lobehub/icons-static-svg` version 1.98.0 (npm), copied
  from its `icons/` folder. Only the four files used here are vendored; the
  package is not a dependency.
- License: MIT (Lobe Icons). The logos themselves are the trademarks of their
  owners (Google, Anthropic, OpenAI, OpenRouter) and are used only to identify
  the provider.
- Files:
  - `gemini.svg`: `gemini-color.svg`, Google Gemini (full colour).
  - `claude.svg`: `claude-color.svg`, Anthropic Claude (full colour).
  - `openai.svg`: OpenAI mark, single colour using `currentColor`.
  - `openrouter.svg`: OpenRouter mark, single colour using `currentColor`.
- Custom (OpenAI-compatible) endpoints have no brand and use a neutral Phosphor
  plug icon instead.

The files are imported with Vite `?raw` by `ui/ai/brand_icon.ts` and rendered
inline, so single-colour marks follow the text colour of the theme.
