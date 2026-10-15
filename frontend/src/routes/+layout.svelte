<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { iniciarSesion, sesion } from '$lib/auth.js';
	import { panelNotificaciones, marcarNotificacionLeida } from '$lib/api.js';

	onMount(() => {
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js');
		}
		iniciarSesion();
	});

	$: ruta = $page.url.pathname;

	// campanita
	let panel = null;
	let abierto = false;

	async function refrescarCampanita() {
		if ($sesion.estado !== 'activa') return;
		try {
			panel = await panelNotificaciones();
		} catch {
			/* silencioso: la campanita no rompe nada */
		}
	}

	$: if ($sesion.estado === 'activa' && !panel) refrescarCampanita();

	onMount(() => {
		const t = setInterval(refrescarCampanita, 30000); // sondeo suave
		return () => clearInterval(t);
	});

	async function abrirNotif(n) {
		abierto = false;
		await marcarNotificacionLeida({ id: n.id });
		panel = null; // fuerza refresco
		if (n.tipo === 'dm_solicitud') goto('/mensajes');
		else if (n.tipo === 'chat_anuncio') goto(`/secciones/${n.ref_id}/chato`);
		else if (n.tipo === 'aviso_importante') goto(`/secciones/${n.ref_id}`);
		else goto('/');
	}

	function grupo(tipo) {
		return {
			dm_solicitud: 'Mensajes',
			chat_anuncio: 'Anuncios de tu sección',
			aviso_importante: 'Importantes',
			anuncio_general: 'Universidad',
			votacion: 'Votaciones'
		}[tipo] || tipo;
	}
</script>

<svelte:head>
	<title>Universidad App</title>
</svelte:head>

<div class="app">
	<header>
		<strong>Universidad App</strong>
		<div class="cabecera-dcha">
			{#if $sesion.estado === 'activa'}
				<div class="campanita">
					<button class="campanita-boton" aria-label="Notificaciones" on:click={() => (abierto = !abierto)}>
						<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
						{#if panel && panel.total_no_leidas > 0}
							<span class="contador">{panel.total_no_leidas > 9 ? '9+' : panel.total_no_leidas}</span>
						{/if}
					</button>
					{#if abierto}
						<div class="panel-notifs">
							{#if panel}
								{#if panel.no_leidas.length === 0 && panel.recientes.length === 0}
									<p class="meta">Sin notificaciones.</p>
								{/if}
								{#if panel.no_leidas.length > 0}
									<p class="grupo">Nuevas</p>
									{#each panel.no_leidas as n}
										<button class="notif no-leida" on:click={() => abrirNotif(n)}>
											<span class="tipo">{grupo(n.tipo)}</span>
											{n.titulo}
										</button>
									{/each}
								{/if}
								{#if panel.recientes.length > 0}
									<p class="grupo">Recientes</p>
									{#each panel.recientes as n}
										<button class="notif leida" on:click={() => abrirNotif(n)}>
											<span class="tipo">{grupo(n.tipo)}</span>
											{n.titulo}
										</button>
									{/each}
								{/if}
							{/if}
						</div>
					{/if}
				</div>
				<span class="rol">{$sesion.usuario?.rol_base}</span>
			{/if}
		</div>
	</header>

	<main><slot /></main>

	{#if $sesion.estado === 'activa'}
		<nav class="inferior">
			<a href="/" class:active={ruta === '/'}>Inicio</a>
			<a href="/mensajes" class:active={ruta.startsWith('/mensajes')}>Mensajes</a>
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
		position: sticky; top: 0; background: #0f172a; z-index: 10;
	}
	.cabecera-dcha { display: flex; align-items: center; gap: 0.8rem; }
	.rol { color: #7dd3fc; font-size: 0.85rem; text-transform: capitalize; }
	main { flex: 1; padding: 1rem 1rem 4.5rem; }

	.campanita { position: relative; }
	.campanita-boton {
		position: relative; background: none; border: 0; color: #94a3b8;
		cursor: pointer; padding: 0.3rem; border-radius: 8px;
	}
	.campanita-boton:hover { color: #e2e8f0; }
	.contador {
		position: absolute; top: -4px; right: -6px; background: #ef4444; color: #fff;
		font-size: 0.65rem; font-weight: 700; border-radius: 999px;
		padding: 0.05rem 0.3rem; min-width: 16px; text-align: center;
	}
	.panel-notifs {
		position: absolute; right: 0; top: 110%; width: 320px; max-height: 70vh; overflow-y: auto;
		background: #1e293b; border: 1px solid #334155; border-radius: 12px;
		padding: 0.6rem; box-shadow: 0 10px 30px rgba(0,0,0,0.5); z-index: 20;
	}
	.grupo { color: #7dd3fc; font-size: 0.72rem; text-transform: uppercase; margin: 0.5rem 0 0.2rem; }
	.notif {
		display: block; width: 100%; text-align: left; padding: 0.5rem 0.6rem;
		border: 0; border-radius: 8px; background: none; color: #e2e8f0;
		cursor: pointer; font: inherit; font-size: 0.9rem;
	}
	.notif .tipo { display: block; color: #64748b; font-size: 0.72rem; }
	.notif.no-leida { background: #0c4a6e; margin-bottom: 0.2rem; }
	.notif.no-leida .tipo { color: #7dd3fc; }
	.notif.leida { opacity: 0.65; }
	.notif:hover { background: #334155; }
	.meta { color: #94a3b8; font-size: 0.85rem; }

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
