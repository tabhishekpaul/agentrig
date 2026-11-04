<script>
	let { size = 24 } = $props();
</script>

<!-- agentrig mark: a rig of three units, each with its status light. Same art as static/icon.svg. -->
<svg width={size} height={size} viewBox="0 0 64 64" aria-label="agentrig" role="img">
	<rect width="64" height="64" fill="#C70036" />
	<g fill="#FFFBF5">
		<rect x="11" y="13" width="42" height="10" rx="1.5" />
		<rect x="11" y="27" width="42" height="10" rx="1.5" />
		<rect x="11" y="41" width="42" height="10" rx="1.5" />
	</g>
	<g fill="#C70036">
		<circle cx="17.5" cy="18" r="2.4" />
		<circle cx="17.5" cy="32" r="2.4" />
		<circle cx="17.5" cy="46" r="2.4" />
		<rect x="31" y="16.75" width="17" height="2.5" rx="1.25" />
		<rect x="31" y="30.75" width="17" height="2.5" rx="1.25" />
		<rect x="31" y="44.75" width="17" height="2.5" rx="1.25" />
	</g>
</svg>
