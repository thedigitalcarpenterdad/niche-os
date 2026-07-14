<script lang="ts" module>
  import type { User } from "../../lib/types";

  // Typing entries decay after this long without a re-ping. Slightly larger
  // than the sender's IDLE_MS so we don't blink off mid-pause.
  export const TYPING_TTL_MS = 3000;

  export type TypingEntry = {
    userID: string;
    user?: User;
    expiresAt: number;
    /** Optional tool activity label, e.g. "searching Gmail" or "querying Notion". */
    toolName?: string;
  };

  /** Human-readable label for a known tool_name value sent by bots. */
  export function toolLabel(toolName: string | undefined): string | undefined {
    if (!toolName) return undefined;
    const map: Record<string, string> = {
      // Web / search
      web_search: "searching the web",
      search_web: "searching the web",
      // Email
      gmail_search: "searching Gmail",
      gmail: "searching Gmail",
      search_gmail: "searching Gmail",
      // Notion
      notion_query: "querying Notion",
      notion: "querying Notion",
      // File system
      read_file: "reading a file",
      write_file: "writing a file",
      list_files: "listing files",
      search_files: "searching files",
      // Terminal / shell
      terminal: "running a command",
      bash: "running a command",
      shell: "running a command",
      // GitHub
      github: "checking GitHub",
      // Calendar
      google_calendar: "checking Calendar",
      calendar: "checking Calendar",
      // Generic fallback shapes
    };
    const key = toolName.toLowerCase().replace(/[^a-z0-9_]/g, "_");
    if (map[key]) return map[key];
    // Fallback: prettify the raw name (underscores → spaces)
    return toolName.replace(/_/g, " ");
  }
</script>

<script lang="ts">
  import type { User } from "../../lib/types";

  type Props = {
    entries: TypingEntry[];
    currentUserID?: string;
  };

  let { entries, currentUserID }: Props = $props();

  let visible = $derived.by(() =>
    entries.filter((entry) => entry.userID !== currentUserID),
  );

  function nameOf(user?: User, fallback = "Someone"): string {
    return user?.display_name?.trim() || (user?.handle ? `@${user.handle}` : fallback);
  }

  function activityOf(entry: TypingEntry): string {
    const activity = toolLabel(entry.toolName);
    return activity ? `is ${activity}…` : "is typing…";
  }

  let label = $derived.by(() => {
    if (visible.length === 0) return "";
    if (visible.length === 1) return `${nameOf(visible[0].user)} ${activityOf(visible[0])}`;
    if (visible.length === 2)
      return `${nameOf(visible[0].user)} and ${nameOf(visible[1].user)} are typing…`;
    if (visible.length === 3)
      return `${nameOf(visible[0].user)}, ${nameOf(visible[1].user)}, and ${nameOf(visible[2].user)} are typing…`;
    return "Several people are typing…";
  });
</script>

<div class="typing-indicator" class:visible={visible.length > 0} aria-live="polite" aria-atomic="true">
  <span class="typing-indicator__dots" aria-hidden="true">
    <i></i><i></i><i></i>
  </span>
  <span class="typing-indicator__label">{label}</span>
</div>
