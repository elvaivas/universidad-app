<script>
	import { onMount } from 'svelte';

	onMount(() => {
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js');
		}
	});
</script>

<svelte:head>
	<title>Universidad App</title>
</svelte:head>

<div class="app">
	<header>
		<strong>Universidad App</strong>
		<nav>
			<a href="/">Inicio</a>
			<a href="/login">Entrar</a>
		</nav>
	</header>
	<main><slot /></main>
</div>

<style>
	:global(body) {
		margin: 0;
		font-family: system-ui, sans-serif;
		background: #0f172a;
		color: #e2e8f0;
	}
	.app { max-width: 720px; margin: 0 auto; min-height: 100vh; display: flex; flex-direction: column; }
	header {
		display: flex; justify-content: space-between; align-items: center;
		padding: 0.8rem 1rem; border-bottom: 1px solid #1e293b;
		position: sticky; top: 0; background: #0f172a;
	}
	header a { color: #7dd3fc; margin-left: 1rem; text-decoration: none; }
	main { flex: 1; padding: 1rem; }
</style>
