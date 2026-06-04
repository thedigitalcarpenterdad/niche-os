<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { page } from "$app/stores";

  let status = "Signing you in...";
  let error = false;

  onMount(async () => {
    const token = $page.url.searchParams.get("token");
    if (!token) { status = "Invalid link — no token found."; error = true; return; }
    try {
      const res = await fetch("/api/auth/magic/consume", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
        credentials: "include",
      });
      if (!res.ok) { const e = await res.json().catch(() => ({})); status = e?.error || "Sign-in failed. The link may have expired."; error = true; return; }
      goto("/");
    } catch (e) {
      status = "Network error. Please try again.";
      error = true;
    }
  });
</script>

<main style="display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;">
  <div style="text-align:center;">
    {#if error}
      <p style="color:#e55;">⚠ {status}</p>
      <a href="/">Go back</a>
    {:else}
      <p>{status}</p>
    {/if}
  </div>
</main>
