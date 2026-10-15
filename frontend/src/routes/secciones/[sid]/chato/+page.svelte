<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { sesion } from '$lib/auth.js';
	import { mensajes, borrarMensaje, conectarChat } from '$lib/api.js';

	const sid = $page.params.sid;

	let canal = 'general'; // general | anuncios
	let historial = [];
	let cargando = true;
	let error = '';
	let intentado = false;

	$: if ($sesion.estado === 'anonimo') goto('/login');
	$: if ($sesion.estado === 'activa' && !intentado) {
		intentado = true;
		conectar();
	}

	let estado = 'conectando'; // conectando | en-vivo | caido
	let texto = '';
	let chat = null;

	async function cargar() {
		cargando = true;
		try {
			const p = await mensajes(sid, { canal });
			historial = Array.isArray(p?.datos) ? p.datos : [];
			scrollAbajo();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	function conectar() {
		chat?.cerrar();
		estado = 'conectando';
		chat = conectarChat(sid, {
			onMensaje: (m) => {
				if (m.canal !== canal) return; // mensaje de la otra pestaña: no interviene
				historial = [...historial, m];
				const vistos = new Set();
				historial = historial.filter((x) => !vistos.has(x.id) && vistos.add(x.id));
				scrollAbajo();
			},
			onBorrado: ({ id }) => {
				historial = historial.filter((m) => m.id !== id);
			},
			onError: (msg) => (error = msg),
			onEstado: (s) => (estado = s)
		});
		cargar();
	}

	async function cambiarCanal(nuevo) {
		canal = nuevo;
		await cargar();
	}

	function enviar() {
		const cuerpo = texto.trim();
		if (!cuerpo) return;
		if (!chat?.enviar(cuerpo, canal)) {
			error = 'conexión caída; reconectando…';
			conectar();
			return;
		}
		texto = '';
		error = '';
	}

	async function borrar(m) {
		try {
			await borrarMensaje(m.id);
		} catch (e) {
			error = e.message;
		}
	}

	function scrollAbajo() {
		requestAnimationFrame(() => {
			const el = document.querySelector('.historial');
			if (el) el.scrollTop = el.scrollHeight;
		});
	}

	function hora(iso) {
		try {
			return new Date(iso).toLocaleTimeString('es-VE', { hour: '2-digit', minute: '2-digit' });
		} catch {
			return '';
		}
	}
</script>

<svelte:head><title>Chat · Universidad App</title></svelte:head>

<a href={`/secciones/${sid}`}>← Novedades</a>

{#if $sesion.estado !== 'activa'}
	<p>Iniciar sesión para entrar al chat.</p>
{:else}
	<div class="barra">
		<h1>Chat</h1>
		<span class="indicador {estado}">
			{estado === 'en-vivo' ? '● en vivo' : estado === 'conectando' ? '… conectando' : '○ caído — reintentando'}
		</span>
	</div>

	<nav class="pestanas">
		<button class:active={canal === 'general'} on:click={() => cambiarCanal('general')}>General</button>
		<button class:active={canal === 'anuncios'} on:click={() => cambiarCanal('anuncios')}>Anuncios</button>
	</nav>

	{#if cargando}<p>Cargando…</p>{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}

	<div class="historial">
		{#each historial || [] as m}
			<div class="fila {m.autor_id === $sesion.usuario?.sub ? 'propio' : ''}">
				<div class="burbuja">
					<span class="meta">
						{m.autor_nombre}
						{#if m.autor_rol}<em class="insignia {m.autor_rol}">{m.autor_rol}</em>{/if}
						· {hora(m.created_at)}
					</span>
					<p>{m.cuerpo}</p>
				</div>
				<button class="borrar" title="Borrar mensaje" on:click={() => borrar(m)}>✕</button>
			</div>
		{/each}
	</div>

	{#if canal === 'anuncios' && !($sesion.usuario?.rol_base === 'profesor' || $sesion.usuario?.es_mantenedor || $sesion.usuario?.voceria === 'clase')}
		<p class="meta">Solo el profesor, un vocero de clase o un mantenedor publican anuncios.</p>
	{/if}
	<form class="entrada" on:submit|preventDefault={enviar}>
		<input bind:value={texto} placeholder={canal === 'anuncios' ? 'Publicar un anuncio…' : 'Escribe un mensaje…'} maxlength="2000" />
		<button type="submit" disabled={!texto.trim()}>{canal === 'anuncios' ? 'Publicar' : 'Enviar'}</button>
	</form>
{/if}

<style>
	.barra { display: flex; justify-content: space-between; align-items: center; }
	.indicador { font-size: 0.85rem; }
	.indicador.en-vivo { color: #4ade80; }
	.indicador.conectando { color: #facc15; }
	.indicador.caido { color: #f87171; }
	.pestanas { display: flex; gap: 0.5rem; margin: 0.6rem 0; }
	.pestanas button {
		padding: 0.4rem 1rem; border-radius: 8px; border: 1px solid #334155;
		background: #0f172a; color: #94a3b8; cursor: pointer; font-weight: 600;
	}
	.pestanas button.active { background: #1e293b; color: #7dd3fc; border-color: #0ea5e9; }
	.historial { display: flex; flex-direction: column; gap: 0.5rem; margin: 0.8rem 0; max-height: 55vh; overflow-y: auto; }
	.fila { display: flex; align-items: flex-start; gap: 0.4rem; }
	.fila.propio { flex-direction: row-reverse; }
	.burbuja { max-width: 80%; padding: 0.5rem 0.8rem; border-radius: 12px; background: #1e293b; }
	.fila.propio .burbuja { background: #0c4a6e; }
	.burbuja p { margin: 0.2rem 0 0; }
	.meta { color: #94a3b8; font-size: 0.75rem; }
	.insignia {
		font-style: normal; font-size: 0.68rem; padding: 0.05rem 0.35rem;
		border-radius: 6px; margin-left: 0.25rem; text-transform: uppercase;
	}
	.insignia.profesor { background: #0c4a6e; color: #7dd3fc; }
	.insignia.vocero { background: #3b0764; color: #d8b4fe; }
	.insignia.mantenedor { background: #7f1d1d; color: #fecaca; }
	.borrar { background: none; border: 0; color: #64748b; cursor: pointer; font-size: 0.8rem; padding: 0.2rem; }
	.borrar:hover { color: #f87171; }
	.entrada { display: flex; gap: 0.5rem; position: sticky; bottom: 4rem; }
	.entrada input { flex: 1; padding: 0.6rem; border-radius: 8px; border: 1px solid #334155; background: #1e293b; color: #e2e8f0; }
	.entrada button { padding: 0.6rem 1.2rem; border: 0; border-radius: 8px; background: #0ea5e9; color: #082f49; font-weight: 700; cursor: pointer; }
	.error { color: #f87171; }
</style>
