<script lang="ts">
  import { enhanceMarkdownGifs } from "../../lib/actions/markdownGifs";
  import { time, markdown } from "../../lib/format";
  import { uploadURL } from "../../lib/uploads";
  import type { Message } from "../../lib/types";
  import MediaAttachment from "../MediaAttachment.svelte";
  import QuoteBlock from "./QuoteBlock.svelte";

  const READ_MORE_THRESHOLD = 500;

  type Props = {
    message: Message;
    index: number;
    selected: boolean;
    replyContext: "channel" | "dm";
    selectedThreadID?: string;
    onReply: (message: Message, context: "channel" | "dm") => void;
    onOpenThread: (message: Message) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
    onRetry?: (message: Message) => void;
    onDiscard?: (message: Message) => void;
  };

  let {
    message,
    index,
    selected,
    replyContext,
    onReply,
    onOpenThread: _onOpenThread,
    onJumpToQuote,
    onOpenImage,
    onRetry,
    onDiscard,
  }: Props = $props();

  let isPending = $derived(message.status === "pending");
  let isFailed = $derived(message.status === "failed");
  let isLong = $derived((message.body?.length ?? 0) > READ_MORE_THRESHOLD);
  let expanded = $state(false);
</script>

<div
  class="message-row"
  class:selected
  class:is-pending={isPending}
  class:is-failed={isFailed}
  data-message-id={message.id}
>
  <span class="row-stamp" aria-hidden="true">{index === 0 ? "" : time(message.created_at)}</span>
  <div class="message-content">
    <QuoteBlock {message} onJump={onJumpToQuote} />
    <div
      class="markdown"
      class:markdown-collapsed={isLong && !expanded}
      use:enhanceMarkdownGifs
    >{@html markdown(message.body)}</div>
    {#if isLong && !expanded}
      <button
        type="button"
        class="read-more-btn"
        onclick={() => (expanded = true)}
      >Read more</button>
    {/if}
    {#if message.attachments?.length}
      <div class="attachment-grid" aria-label="Attachments">
        {#each message.attachments as attachment (attachment.id)}
          <MediaAttachment
            upload={attachment}
            url={uploadURL(attachment)}
            onOpenImage={onOpenImage}
          />
        {/each}
      </div>
    {/if}
    {#if isFailed}
      <div class="message-failed" role="alert">
        <span class="message-failed__label">Couldn't send.</span>
        {#if onRetry}
          <button type="button" class="message-failed__action" onclick={() => onRetry?.(message)}>Retry</button>
        {/if}
        {#if onDiscard}
          <button type="button" class="message-failed__action message-failed__action--ghost" onclick={() => onDiscard?.(message)}>Discard</button>
        {/if}
      </div>
    {/if}
  </div>
  <div class="message-actions" aria-label="Message actions">
    <button
      type="button"
      aria-label="Reply"
      class="tooltip"
      data-tooltip="Reply"
      disabled={isPending || isFailed}
      onclick={() => onReply(message, replyContext)}
    >
      <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
        <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M9 17 4 12l5-5M4 12h11a5 5 0 0 1 5 5v3"/>
      </svg>
    </button>
  </div>
</div>

<style>
  .markdown-collapsed {
    max-height: 12rem;
    overflow: hidden;
    position: relative;
  }
  .markdown-collapsed::after {
    content: "";
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 3rem;
    background: linear-gradient(to bottom, transparent, var(--bg-primary, #1a1a1a));
    pointer-events: none;
  }
  .read-more-btn {
    display: inline-block;
    margin-top: 0.25rem;
    padding: 0;
    background: none;
    border: none;
    color: var(--accent, #7c8cf8);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .read-more-btn:hover {
    opacity: 0.8;
  }
</style>
