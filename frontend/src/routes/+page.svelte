<script>
	import { onMount } from 'svelte';
	import { sesion } from '$lib/auth.js';
	import { misSecciones } from '$lib/api.js';

	let secciones = [];
	let cargando = false;
	let error = '';

	onMount(async () => {
		if ($sesion.estado !== 'activa') return;
		cargando = true;
		try {
			secciones = await misSecciones();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	});
</script>

<svelte:head><title>Inicio · Universidad App</title></svelte:head>

{#if $sesion.estado === 'cargando'}
	<p>Cargando…</p>
{:else if $sesion.estado === 'activa'}
	<h1>Mis secciones</h1>
	{#if cargando}
		<p>Cargando…</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if secciones.length === 0}
		<p>No estás inscrito en ninguna sección todavía.</p>
	{:else}
		<ul class="lista">
			{#each secciones as s}
				<li>
					<a href={`/secciones/${s.id}`}>
						<strong>{s.materia_nombre}</strong>
						<span>Sección {s.codigo} · {s.periodo_nombre}{s.es_profesor ? ' · profesor' : ''}</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
{:else}
	<h1>Bienvenido</h1>
	<p><a href="/login">Iniciar sesión</a></p>
{/if}

<style>
	.lista { list-style: none; padding: 0; }
	.lista li { margin: 0.6rem 0; }
	.lista a {
		display: block; padding: 0.9rem 1rem; border-radius: 10px;
		background: #1e293b; text-decoration: none; color: #e2e8f0;
	}
	.lista a span { display: block; color: #94a3b8; font-size: 0.9rem; margin-top: 0.2rem; }
	.error { color: #f87171; }
</style>
