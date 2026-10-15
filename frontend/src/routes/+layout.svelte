<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { iniciarSesion, sesion } from '$lib/auth.js';

	onMount(() => {
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js');
		}
		iniciarSesion();
	});

	$: ruta = $page.url.pathname;
</script>

<svelte:head>
	<title>Universidad App</title>
</svelte:head>

<div class="app">
	<header>
		<strong>Universidad App</strong>
		{#if $sesion.estado === 'activa'}
			<span class="rol">{$sesion.usuario?.rol_base}</span>
		{/if}
	</header>

	<main><slot /></main>

	{#if $sesion.estado === 'activa'}
		<nav class="inferior">
			<a href="/" class:active={ruta === '/'}>Inicio</a>
			<a href="/perfil" class:active={ruta === '/perfil'}>Perfil</a>
		</nav>
	{/if}
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
		position: sticky; top: 0; background: #0f172a; z-index: 1;
	}
	.rol { color: #7dd3fc; font-size: 0.85rem; text-transform: capitalize; }
	main { flex: 1; padding: 1rem 1rem 4.5rem; }
	.inferior {
		position: fixed; bottom: 0; left: 0; right: 0;
		display: flex; justify-content: space-around;
		background: #1e293b; border-top: 1px solid #334155;
		padding: 0.6rem 0 max(0.6rem, env(safe-area-inset-bottom));
		max-width: 720px; margin: 0 auto;
	}
	.inferior a {
		color: #94a3b8; text-decoration: none; font-weight: 600; padding: 0.3rem 1rem;
		border-radius: 8px;
	}
	.inferior a.active { color: #7dd3fc; background: #0f172a; }
</style>
