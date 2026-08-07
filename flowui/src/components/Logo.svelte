<script lang="ts">
  // The FlowControl mark: one node unblocks two. Drawn with currentColor so the
  // parent chooses the tint (var(--accent)) and it inverts with the theme.
  let { size = 26 }: { size?: number } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 64 64"
  role="img"
  aria-label="FlowControl">
  <g fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="butt">
    <path d="M22.7,28.9 L41.2,19"></path>
    <path d="M22.7,35.1 L41.2,45"></path>
  </g>
  <circle cx="16" cy="32" r="8" fill="currentColor" stroke="none"></circle>
  <circle cx="47.5" cy="16.5" r="7.5" fill="none" stroke="currentColor" stroke-width="3"></circle>
  <circle cx="47.5" cy="47.5" r="7.5" fill="none" stroke="currentColor" stroke-width="3"></circle>
</svg>
