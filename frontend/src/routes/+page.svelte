<script>
	import { sesion } from '$lib/auth.js';
</script>

<svelte:head><title>Inicio · Universidad App</title></svelte:head>

{#if $sesion.estado === 'cargando'}
	<p>Cargando…</p>
{:else if $sesion.estado === 'activa'}
	<h1>Hola, {$sesion.usuario?.rol_base}</h1>
	<p>Sesión activa. Secciones: {$sesion.usuario?.seccion_ids?.length ?? 0}.</p>
{:else}
	<h1>Bienvenido</h1>
	<p><a href="/login">Iniciar sesión</a></p>
{/if}
