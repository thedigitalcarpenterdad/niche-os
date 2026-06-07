<script lang="ts">
  import { tick } from "svelte";
  import { autoGrow } from "../../lib/actions/autogrow";
  import { avatarInitial, handleLabel } from "../../lib/chat/people";
  import { formatBytes, isImageUpload, uploadURL } from "../../lib/uploads";
  import type { GifItem } from "../../lib/gifs";
  import type { Message, SlashCommand, Upload, User } from "../../lib/types";
  import ComposerToolbar from "./ComposerToolbar.svelte";
  import GifPicker from "./GifPicker.svelte";
  import ReplyPreview from "./ReplyPreview.svelte";

  const OPENAI_API_KEY = import.meta.env.VITE_OPENAI_API_KEY as string | undefined;

  type ActiveToken = {
    kind: "slash" | "mention";
    start: number;
    end: number;
    query: string;
    raw: string;
  };

  type ComposerSuggestion = {
    id: string;
    kind: "slash" | "mention";
    label: string;
    detail: string;
    insertText: string;
    sortText: string;
  };

  type Props = {
    value: string;
    placeholder: string;
    ariaLabel: string;
    submitLabel: string;
    formClass?: string;
    pendingUpload?: Upload | null;
    replyTarget?: Message | null;
    showUpload?: boolean;
    showToolbar?: boolean;
    showGifPicker?: boolean;
    gifQuery?: string;
    filteredGifs?: GifItem[];
    slashCommands?: SlashCommand[];
    mentionPeople?: User[];
    onValue: (value: string) => void;
    onSubmit: () => void;
    onKeydown: (event: KeyboardEvent) => void;
    onFocus: () => void;
    onInputRef: (node: HTMLTextAreaElement | null) => void;
    onUploadFile?: (event: Event) => void;
    onRemoveUpload?: () => void;
    onClearReply?: () => void;
    onApplyMarkdownWrap?: (before: string, after?: string) => void;
    onAppendToComposer?: (snippet: string) => void;
    onToggleGif?: () => void;
    onGifQuery?: (value: string) => void;
    onPickGif?: (url: string, title: string) => void;
    onVoiceNote?: (blob: Blob, durationMs: number) => void;
  };

  let {
    value,
    placeholder,
    ariaLabel,
    submitLabel,
    formClass = "composer",
    pendingUpload = null,
    replyTarget = null,
    showUpload = false,
    showToolbar = false,
    showGifPicker = false,
    gifQuery = "",
    filteredGifs = [],
    slashCommands = [],
    mentionPeople = [],
    onValue,
    onSubmit,
    onKeydown,
    onFocus,
    onInputRef,
    onUploadFile = () => {},
    onRemoveUpload = () => {},
    onClearReply = () => {},
    onApplyMarkdownWrap = () => {},
    onAppendToComposer = () => {},
    onToggleGif = () => {},
    onGifQuery = () => {},
    onPickGif = () => {},
    onVoiceNote,
  }: Props = $props();

  let input: HTMLTextAreaElement | null = $state(null);
  let caret = $state(0);
  let dismissedToken = $state("");
  let selectedSuggestionIndex = $state(0);

  // Voice recording state
  let recording = $state(false);
  let recordingMs = $state(0);
  let recMR: MediaRecorder | null = null;
  let recStream: MediaStream | null = null;
  let recChunks: Blob[] = [];
  let recInterval: ReturnType<typeof setInterval> | null = null;

  // Voice note pending state (stopped, not yet sent/transcribed)
  let pendingBlob: Blob | null = $state(null);
  let pendingDurMs = $state(0);
  let transcribing = $state(false);
  let transcribeError = $state("");

  const activeToken = $derived.by(() => detectActiveToken(value, caret));
  const activeSuggestions = $derived.by(() => {
    if (!activeToken || tokenKey(activeToken) === dismissedToken) return [];
    return activeToken.kind === "slash"
      ? slashSuggestions(activeToken)
      : mentionSuggestions(activeToken);
  });

  $effect(() => {
    onInputRef(input);
    return () => onInputRef(null);
  });

  $effect(() => {
    if (activeSuggestions.length === 0) {
      selectedSuggestionIndex = 0;
      return;
    }
    if (selectedSuggestionIndex >= activeSuggestions.length) selectedSuggestionIndex = 0;
  });

  function detectActiveToken(text: string, position: number): ActiveToken | null {
    const safePosition = Math.max(0, Math.min(position || text.length, text.length));
    const before = text.slice(0, safePosition);
    const match = /(^|\s)([/@][^\s]*)$/.exec(before);
    if (!match) return null;
    const raw = match[2];
    const start = before.length - raw.length;
    if (raw.startsWith("/") && start !== 0) return null;
    return {
      kind: raw.startsWith("/") ? "slash" : "mention",
      start,
      end: safePosition,
      query: raw.slice(1).toLowerCase(),
      raw,
    };
  }

  function tokenKey(token: ActiveToken): string {
    return `${token.kind}:${token.start}:${token.raw}`;
  }

  function updateCaret(node: HTMLTextAreaElement | null = input) {
    caret = node?.selectionStart ?? value.length;
  }

  function normalizedCommand(command: string): string {
    return command.startsWith("/") ? command : `/${command}`;
  }

  function slashSuggestions(token: ActiveToken): ComposerSuggestion[] {
    const query = token.query;
    return slashCommands
      .filter((command) => !command.revoked_at)
      .map((command) => {
        const label = normalizedCommand(command.command);
        const searchable = label.slice(1).toLowerCase();
        return {
          id: command.id,
          kind: "slash" as const,
          label,
          detail: command.description || "Slash command",
          insertText: `${label} `,
          sortText: searchable,
        };
      })
      .filter((suggestion) => !query || suggestion.sortText.includes(query))
      .sort((a, b) => Number(!a.sortText.startsWith(query)) - Number(!b.sortText.startsWith(query)) || a.sortText.localeCompare(b.sortText))
      .slice(0, 6);
  }

  function mentionText(person: User): string {
    return handleLabel(person.handle || person.display_name.replace(/\s+/g, ""));
  }

  function mentionSuggestions(token: ActiveToken): ComposerSuggestion[] {
    const query = token.query;
    const seen = new Set<string>();
    return mentionPeople
      .filter((person) => {
        if (!person.id || seen.has(person.id)) return false;
        seen.add(person.id);
        return true;
      })
      .map((person) => {
        const label = mentionText(person);
        const searchable = `${person.handle || ""} ${person.display_name}`.trim().toLowerCase();
        return {
          id: person.id,
          kind: "mention" as const,
          label,
          detail: person.kind === "bot" ? `${person.display_name} · bot` : person.display_name,
          insertText: `${label} `,
          sortText: searchable,
        };
      })
      .filter((suggestion) => !query || suggestion.sortText.includes(query))
      .sort((a, b) => Number(!a.sortText.startsWith(query)) - Number(!b.sortText.startsWith(query)) || a.sortText.localeCompare(b.sortText))
      .slice(0, 6);
  }

  function pickSuggestion(suggestion: ComposerSuggestion) {
    if (!activeToken) return;
    const nextValue = `${value.slice(0, activeToken.start)}${suggestion.insertText}${value.slice(activeToken.end)}`;
    const nextCaret = activeToken.start + suggestion.insertText.length;
    onValue(nextValue);
    void tick().then(() => {
      input?.focus();
      input?.setSelectionRange(nextCaret, nextCaret);
      caret = nextCaret;
    });
  }

  function handleInput(event: Event) {
    const node = event.currentTarget as HTMLTextAreaElement;
    onValue(node.value);
    updateCaret(node);
  }

  function handleFocus() {
    updateCaret();
    onFocus();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (activeSuggestions.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        selectedSuggestionIndex = (selectedSuggestionIndex + 1) % activeSuggestions.length;
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        selectedSuggestionIndex = (selectedSuggestionIndex - 1 + activeSuggestions.length) % activeSuggestions.length;
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        pickSuggestion(activeSuggestions[selectedSuggestionIndex]);
        return;
      }
      if (event.key === "Escape" && activeToken) {
        event.preventDefault();
        dismissedToken = tokenKey(activeToken);
        return;
      }
    }
    onKeydown(event);
  }

  // Voice recording functions
  async function startRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      recStream = stream;
      const mimeType = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus', 'audio/webm', 'audio/mp4'].find(t => MediaRecorder.isTypeSupported(t)) ?? '';
      recChunks = [];
      recordingMs = 0;
      const mr = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
      recMR = mr;
      mr.ondataavailable = (e) => { if (e.data.size > 0) recChunks.push(e.data); };
      mr.start(200);
      recording = true;
      recInterval = setInterval(() => { recordingMs += 100; }, 100);
    } catch (e) {
      console.error('Mic denied:', e);
    }
  }

  function stopRecording() {
    if (!recMR) return;
    const dur = recordingMs;
    const mime = recMR.mimeType || 'audio/webm';
    recMR.onstop = () => {
      recStream?.getTracks().forEach(t => t.stop());
      recStream = null;
      const blob = new Blob(recChunks, { type: mime });
      pendingBlob = blob;
      pendingDurMs = dur;
      recording = false;
      recChunks = [];
      recMR = null;
    };
    recMR.stop();
    if (recInterval) { clearInterval(recInterval); recInterval = null; }
  }

  function sendVoiceNote() {
    if (!pendingBlob) return;
    onVoiceNote?.(pendingBlob, pendingDurMs);
    pendingBlob = null;
    pendingDurMs = 0;
    transcribeError = "";
    recordingMs = 0;
  }

  function discardPending() {
    pendingBlob = null;
    pendingDurMs = 0;
    transcribeError = "";
    recordingMs = 0;
  }

  function cancelRecording() {
    if (recMR) {
      recMR.ondataavailable = null;
      recMR.onstop = null;
      try { recMR.stop(); } catch {}
      recMR = null;
    }
    recStream?.getTracks().forEach(t => t.stop());
    recStream = null;
    if (recInterval) { clearInterval(recInterval); recInterval = null; }
    recording = false;
    recordingMs = 0;
    recChunks = [];
  }

  async function transcribeAudio() {
    if (!pendingBlob || transcribing) return;
    transcribing = true;
    transcribeError = "";
    try {
      const key = OPENAI_API_KEY;
      if (!key) throw new Error("No API key configured");
      const ext = pendingBlob.type.includes("ogg") ? "ogg" : "webm";
      const formData = new FormData();
      formData.append("file", new File([pendingBlob], `voice.${ext}`, { type: pendingBlob.type }));
      formData.append("model", "whisper-1");
      const res = await fetch("https://api.openai.com/v1/audio/transcriptions", {
        method: "POST",
        headers: { Authorization: `Bearer ${key}` },
        body: formData,
      });
      if (!res.ok) {
        const err = await res.text().catch(() => `HTTP ${res.status}`);
        throw new Error(err);
      }
      const data = await res.json() as { text?: string };
      const transcript = data.text?.trim() ?? "";
      onValue(transcript);
      pendingBlob = null;
      pendingDurMs = 0;
      recordingMs = 0;
      await tick();
      input?.focus();
      const len = transcript.length;
      input?.setSelectionRange(len, len);
      caret = len;
    } catch (e: unknown) {
      console.error("Transcription error:", e);
      transcribeError = "Transcription failed — try again or send as voice note";
    } finally {
      transcribing = false;
    }
  }

  function formatRecordingTime(ms: number): string {
    const s = Math.floor(ms / 1000);
    return `${Math.floor(s / 60)}:${(s % 60).toString().padStart(2, '0')}`;
  }
</script>

<form
  class={formClass}
  onsubmit={(event) => {
    event.preventDefault();
    onSubmit();
  }}
>
  {#if showGifPicker}
    <GifPicker
      gifs={filteredGifs}
      query={gifQuery}
      onQuery={onGifQuery}
      onPick={onPickGif}
    />
  {/if}
  {#if activeSuggestions.length > 0}
    <div class="composer-suggestions" role="listbox" aria-label={activeToken?.kind === "slash" ? "Slash command suggestions" : "Mention suggestions"}>
      {#each activeSuggestions as suggestion, index (suggestion.id)}
        <button
          type="button"
          class:active={index === selectedSuggestionIndex}
          role="option"
          aria-selected={index === selectedSuggestionIndex}
          onmousedown={(event) => event.preventDefault()}
          onclick={() => pickSuggestion(suggestion)}
        >
          <span class="suggestion-mark" aria-hidden="true">
            {#if suggestion.kind === "slash"}
              /
            {:else}
              {avatarInitial(suggestion.detail)}
            {/if}
          </span>
          <span class="suggestion-copy">
            <strong>{suggestion.label}</strong>
            <span>{suggestion.detail}</span>
          </span>
          <span class="suggestion-kind">{suggestion.kind === "slash" ? "command" : "mention"}</span>
        </button>
      {/each}
    </div>
  {/if}
  <div class="composer-card">
    {#if pendingUpload}
      <div class="composer-attachment">
        <span class="attachment-icon" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="14" height="14"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/></svg>
        </span>
        {#if isImageUpload(pendingUpload)}
          <img class="pending-image" src={uploadURL(pendingUpload)} alt={pendingUpload.filename} />
        {/if}
        <span class="attachment-name">{pendingUpload.filename} · {formatBytes(pendingUpload.byte_size)}</span>
        <button type="button" class="attachment-remove" aria-label="Remove attachment" onclick={onRemoveUpload}>×</button>
      </div>
    {/if}
    {#if replyTarget}
      <ReplyPreview target={replyTarget} onClear={onClearReply} />
    {/if}
    <div class="composer-row" class:is-recording={recording} class:is-voice-preview={!!pendingBlob}>
      {#if recording}
        <!-- RECORDING STATE -->
        <button type="button" class="composer-icon voice-cancel-btn" onclick={cancelRecording} title="Cancel recording" aria-label="Cancel recording">
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <path fill="currentColor" d="M18 6 6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
        </button>
        <div class="voice-recording-bar">
          <span class="voice-pulse" aria-hidden="true"></span>
          <span class="voice-timer">{formatRecordingTime(recordingMs)}</span>
          <span class="voice-label">Recording…</span>
        </div>
        <button type="button" class="send" onclick={stopRecording} title="Stop recording" aria-label="Stop recording">
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <rect x="5" y="5" width="14" height="14" rx="2" fill="currentColor"/>
          </svg>
        </button>
      {:else if pendingBlob}
        <!-- VOICE NOTE PREVIEW STATE -->
        <button type="button" class="composer-icon voice-cancel-btn" onclick={discardPending} title="Discard voice note" aria-label="Discard voice note">
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <path fill="currentColor" d="M18 6 6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
        </button>
        <div class="voice-recording-bar">
          {#if transcribing}
            <span class="voice-spinner" aria-hidden="true"></span>
            <span class="voice-label">Transcribing…</span>
          {:else}
            <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true" style="opacity:0.6;flex-shrink:0">
              <rect x="9" y="2" width="6" height="11" rx="3" fill="currentColor"/>
              <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M5 10a7 7 0 0 0 14 0M12 19v3"/>
            </svg>
            <span class="voice-timer">{formatRecordingTime(pendingDurMs)}</span>
            {#if transcribeError}
              <span class="voice-error">{transcribeError}</span>
            {:else}
              <span class="voice-label">Voice note ready</span>
            {/if}
          {/if}
        </div>
        {#if !transcribing}
          <button
            type="button"
            class="voice-transcribe-btn"
            onclick={transcribeAudio}
            title="Transcribe to text"
            aria-label="Transcribe to text"
          >
            <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
              <path fill="currentColor" d="M3 7h18M3 12h12M3 17h9"/>
            </svg>
            <span>Transcribe</span>
          </button>
          <button type="button" class="send" onclick={sendVoiceNote} title="Send voice note" aria-label="Send voice note">
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
              <path fill="currentColor" d="M3 3.5 21 12 3 20.5l3.6-7.5L15 12 6.6 11l-3.6-7.5Z"/>
            </svg>
          </button>
        {/if}
      {:else}
        <!-- NORMAL TEXT STATE -->
        {#if showUpload}
          <label class="composer-icon" title="Upload file">
            <input type="file" aria-label="Upload file" onchange={onUploadFile} />
            <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
              <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/>
            </svg>
          </label>
        {/if}
        <textarea
          bind:this={input}
          value={value}
          use:autoGrow={value}
          rows="1"
          {placeholder}
          aria-label={ariaLabel}
          oninput={handleInput}
          onfocus={handleFocus}
          onkeydown={handleKeydown}
          onkeyup={() => updateCaret()}
          onmouseup={() => updateCaret()}
          onselect={() => updateCaret()}
        ></textarea>
        {#if value.trim()}
          <button type="submit" class="send" aria-label={submitLabel} disabled={!value.trim()}>
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
              <path fill="currentColor" d="M3 3.5 21 12 3 20.5l3.6-7.5L15 12 6.6 11l-3.6-7.5Z"/>
            </svg>
          </button>
        {:else}
          <button type="button" class="send mic-btn" onclick={startRecording} title="Record voice note" aria-label="Record voice note">
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
              <rect x="9" y="2" width="6" height="11" rx="3" fill="currentColor"/>
              <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M5 10a7 7 0 0 0 14 0M12 19v3"/>
            </svg>
          </button>
        {/if}
      {/if}
    </div>
    {#if showToolbar}
      <ComposerToolbar
        showGifPicker={showGifPicker}
        onWrap={onApplyMarkdownWrap}
        onAppend={onAppendToComposer}
        onToggleGif={onToggleGif}
      />
    {/if}
  </div>
</form>
