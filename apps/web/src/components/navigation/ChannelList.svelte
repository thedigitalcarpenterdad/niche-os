<script lang="ts">
  import type { Channel, Topic } from "../../lib/types";

  type Props = {
    channels: Channel[];
    topics: Topic[];
    selectedChannelID: string;
    selectedDirectID: string;
    selectedTopicID: string;
    hrefForChannel: (channelID: string) => string;
    onSelectChannel: (channelID: string) => void;
    onSelectTopic: (channelID: string, topicID: string) => void;
    onCreateChannel: () => void;
  };

  let {
    channels,
    topics,
    selectedChannelID,
    selectedDirectID,
    selectedTopicID,
    hrefForChannel,
    onSelectChannel,
    onSelectTopic,
    onCreateChannel,
  }: Props = $props();

  // Channels are hidden once archived (archived_at is set by the API).
  const visibleChannels = $derived(channels.filter((channel) => !channel.archived_at));

  // Group active topics by channel for quick lookup.
  const topicsByChannel = $derived.by(() => {
    const map = new Map<string, Topic[]>();
    for (const topic of topics) {
      if (topic.archived_at) continue;
      if (!topic.channel_id) continue;
      const list = map.get(topic.channel_id) ?? [];
      list.push(topic);
      map.set(topic.channel_id, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => a.name.localeCompare(b.name));
    }
    return map;
  });

  // Channels with topics start expanded; users can collapse them.
  let collapsed = $state<Record<string, boolean>>({});

  function toggle(channelID: string) {
    collapsed = { ...collapsed, [channelID]: !collapsed[channelID] };
  }

  function shouldHandleClientNavigation(event: MouseEvent): boolean {
    return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
  }
</script>

<section class="nav-section">
  <div class="section-title">
    <span class="caret" aria-hidden="true">▾</span>
    <span class="label">Channels</span>
    <button
      type="button"
      class="add-button"
      aria-label="Create channel"
      title="Create channel"
      onclick={onCreateChannel}
    >＋</button>
  </div>
  <div class="nav-list">
    {#each visibleChannels as channel (channel.id)}
      {@const unread = channel.unread_count || 0}
      {@const channelTopics = topicsByChannel.get(channel.id) ?? []}
      {@const isChannelActive = channel.id === selectedChannelID && !selectedDirectID && !selectedTopicID}
      {@const isOpen = channelTopics.length > 0 && !collapsed[channel.id]}
      <div class="channel-group">
        <a
          href={hrefForChannel(channel.id)}
          class="nav-item channel"
          class:active={isChannelActive}
          class:has-unread={unread > 0 && !isChannelActive}
          onclick={(event) => {
            if (!shouldHandleClientNavigation(event)) return;
            event.preventDefault();
            onSelectChannel(channel.id);
          }}
        >
          {#if channelTopics.length > 0}
            <button
              type="button"
              class="topic-caret"
              class:open={isOpen}
              aria-label={isOpen ? `Collapse ${channel.name} topics` : `Expand ${channel.name} topics`}
              onclick={(event) => {
                event.preventDefault();
                event.stopPropagation();
                toggle(channel.id);
              }}
            >▸</button>
          {:else}
            <span class="topic-caret-spacer" aria-hidden="true"></span>
          {/if}
          <span class="hash">#</span> <span class="nav-label">{channel.name}</span>
          {#if unread > 0 && !isChannelActive}
            <span class="unread-badge" aria-label={`${unread} unread`}>{unread > 99 ? "99+" : unread}</span>
          {/if}
        </a>

        {#if isOpen}
          <div class="topic-list">
            {#each channelTopics as topic (topic.id)}
              <a
                href={hrefForChannel(channel.id)}
                class="nav-item topic"
                class:active={topic.id === selectedTopicID && channel.id === selectedChannelID && !selectedDirectID}
                onclick={(event) => {
                  if (!shouldHandleClientNavigation(event)) return;
                  event.preventDefault();
                  onSelectTopic(channel.id, topic.id);
                }}
                title={topic.name}
              >
                <span class="topic-bullet" aria-hidden="true"># </span>
                <span class="nav-label">{topic.name}</span>
              </a>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
    {#if visibleChannels.length === 0}
      <p class="nav-empty">No channels yet</p>
    {/if}
  </div>
</section>

<style>
  .channel-group {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .topic-caret {
    border: 0;
    background: transparent;
    color: var(--muted-2);
    width: 14px;
    height: 14px;
    flex-shrink: 0;
    display: grid;
    place-items: center;
    font-size: 10px;
    line-height: 1;
    cursor: pointer;
    padding: 0;
    border-radius: 3px;
    transition: transform 120ms ease, color 100ms ease;
  }

  .topic-caret.open {
    transform: rotate(90deg);
  }

  .topic-caret:hover {
    color: var(--text);
    background: var(--hover);
  }

  .topic-caret-spacer {
    width: 14px;
    flex-shrink: 0;
  }

  .topic-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    margin-left: 18px;
    border-left: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    padding-left: 4px;
  }

  .nav-item.topic {
    min-height: 26px;
    padding: 4px 8px;
    font-size: 13px;
  }

  .nav-item.topic .nav-label {
    font-size: 13px;
  }

  .topic-bullet {
    color: var(--muted-2);
    font-weight: 600;
    flex-shrink: 0;
    font-size: 12px;
  }

  .nav-item.topic.active .topic-bullet {
    color: var(--accent);
  }
</style>
