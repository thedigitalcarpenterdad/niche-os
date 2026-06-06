<script lang="ts">
  import Avatar from "../avatar/Avatar.svelte";
  import { avatarHue, directConversationForUser, handleLabel } from "../../lib/chat/people";
  import type { Channel, DirectConversation, Topic, User } from "../../lib/types";
  import ChannelList from "./ChannelList.svelte";
  import DirectMessageList from "./DirectMessageList.svelte";

  type Props = {
    workspaceName?: string;
    status: string;
    connected: boolean;
    sidebarCollapsed: boolean;
    channels: Channel[];
    topics: Topic[];
    directConversations: DirectConversation[];
    recentPeople: User[];
    currentUser: User | null;
    selectedChannelID: string;
    selectedDirectID: string;
    selectedTopicID: string;
    selectedProfile: User | null;
    onToggleCollapse: () => void;
    hrefForChannel: (channelID: string) => string;
    hrefForDirect: (conversationID: string) => string;
    onSelectChannel: (channelID: string) => void;
    onSelectTopic: (channelID: string, topicID: string) => void;
    onCreateChannel: () => void;
    onSelectDirect: (conversationID: string) => void;
    onCreateDirect: () => void;
    onOpenProfile: (profile: User) => void;
    onOpenSettings: () => void;
    bots?: User[];
    onVoiceAgent?: (bot: any) => void;
    onAgentChat?: (agent: any) => void;
  };

  let {
    workspaceName,
    status,
    connected,
    sidebarCollapsed,
    channels,
    topics,
    directConversations,
    recentPeople,
    currentUser,
    selectedChannelID,
    selectedDirectID,
    selectedTopicID,
    selectedProfile,
    onToggleCollapse,
    hrefForChannel,
    hrefForDirect,
    onSelectChannel,
    onSelectTopic,
    onCreateChannel,
    onSelectDirect,
    onCreateDirect,
    onOpenProfile,
    onOpenSettings,
    bots = [],
    onVoiceAgent = () => {},
    onAgentChat = () => {},
  }: Props = $props();


  const AGENTS = [
    { id: 'builder',    name: 'Builder AI',        icon: '🏗️', desc: 'Job site questions, specs, drawings, docs', voice: true,  talkieSlug: 'builder-internal' },
    { id: 'estimating', name: 'Estimating Agent',  icon: '📋', desc: 'Takeoffs, bid analysis, scope review',      voice: false, talkieSlug: '' },
    { id: 'rfp-scout',  name: 'RFP Scout',          icon: '🔍', desc: 'FISP leads, new RFPs, deadline alerts',     voice: false, talkieSlug: '' },
    { id: 'daily-brief',name: 'Daily Brief',        icon: '📰', desc: 'Morning project summary on demand',         voice: false, talkieSlug: '' },
    { id: 'sub-manager',name: 'Sub Manager',        icon: '🔧', desc: 'Subcontractor comms and tracking',          voice: false, talkieSlug: '' },
  ];

  function shouldHandleClientNavigation(event: MouseEvent): boolean {
    return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
  }
</script>

<aside class="sidebar" aria-label="Channels and DMs">
  <header class="workspace-header">
    <div class="workspace-name">
      <strong>{workspaceName || "Pick a workspace"}</strong>
      <span class="presence" class:online={connected}>{connected ? "Connected" : status}</span>
    </div>
    <button
      type="button"
      class="sidebar-collapse"
      aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
      title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
      onclick={onToggleCollapse}
    >
      <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
        <path
          fill="none"
          stroke="currentColor"
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d={sidebarCollapsed ? "m9 6 6 6-6 6" : "m15 6-6 6 6 6"}
        />
      </svg>
    </button>
  </header>

  <div class="sidebar-scroll">
    <ChannelList
      {channels}
      {topics}
      {selectedChannelID}
      {selectedDirectID}
      {selectedTopicID}
      {hrefForChannel}
      {onSelectChannel}
      {onSelectTopic}
      {onCreateChannel}
    />

    <DirectMessageList
      conversations={directConversations}
      currentUserID={currentUser?.id}
      {selectedDirectID}
      {hrefForDirect}
      {onSelectDirect}
      {onCreateDirect}
    />

    <section class="agents-section">
      <div class="section-title">
        <span class="caret" aria-hidden="true">▾</span>
        <span class="label">AI Agents</span>
      </div>
      <div class="agents-list">
        {#each AGENTS as agent}
          <div class="agent-card">
            <div class="agent-card-icon">{agent.icon}</div>
            <div class="agent-card-body">
              <span class="agent-card-name">{agent.name}</span>
              <span class="agent-card-desc">{agent.desc}</span>
              <div class="agent-card-actions">
                <button
                  class="agent-action-btn"
                  type="button"
                  onclick={() => onAgentChat(agent)}
                >💬 Chat</button>
                {#if agent.voice}
                  <button
                    class="agent-action-btn agent-action-voice"
                    type="button"
                    onclick={() => onVoiceAgent(agent)}
                  >🎙️ Voice</button>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    </section>

    <section class="nav-section">
      <div class="section-title">
        <span class="caret" aria-hidden="true">▾</span>
        <span class="label">People</span>
      </div>
      <div class="nav-list">
        {#each recentPeople as person (person.id)}
          {@const conversation = directConversationForUser(directConversations, person.id)}
          <a
            href={conversation ? hrefForDirect(conversation.id) : "#"}
            class="nav-item dm"
            class:active={conversation?.id === selectedDirectID || selectedProfile?.id === person.id}
            onclick={(event) => {
              if (conversation) {
                if (!shouldHandleClientNavigation(event)) return;
                event.preventDefault();
                onSelectDirect(conversation.id);
              } else {
                event.preventDefault();
                onOpenProfile(person);
              }
            }}
          >
            <Avatar
              class="dm-avatar"
              id={person.id}
              name={person.display_name}
              src={person.avatar_url}
              size={22}
            />
            <span class="nav-label">{person.display_name}</span>
            <span class="presence-dot active" aria-hidden="true"></span>
          </a>
        {/each}
        {#if recentPeople.length === 0}
          <p class="nav-empty">People appear here as you chat</p>
        {/if}
      </div>
    </section>
  </div>

  {#if currentUser}
    <button
      class="user-card"
      type="button"
      onclick={onOpenSettings}
      oncontextmenu={(event) => {
        event.preventDefault();
        onOpenSettings();
      }}
      aria-label={`Account settings for ${currentUser.display_name} ${handleLabel(currentUser.handle)}`}
    >
      <Avatar
        class="dm-avatar"
        id={currentUser.id}
        name={currentUser.display_name}
        src={currentUser.avatar_url}
        size={28}
        loading="eager"
        fetchPriority="auto"
      />
      <div class="user-meta">
        <strong>{currentUser.display_name}</strong>
        <span>{currentUser.handle ? handleLabel(currentUser.handle) : connected ? "Active" : "Reconnecting…"}</span>
      </div>
      <span class="presence-dot active" aria-hidden="true"></span>
    </button>
    <a href="/api/auth/logout" class="sign-out-btn" title="Sign out" aria-label="Sign out">&#x23FB;</a>
  {/if}
</aside>
