<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { sesion } from '$lib/auth.js';
	import { mensajesDM, borrarMensajeDM, conectarDM } from '$lib/api.js';

	const cid = $page.params.cid;

	let historial = [];
	let cargando = true;
	let error = '';
	let intentado = false;

	$: if ($sesion.estado === 'anonimo') goto('/login');
	$: if ($sesion.estado === 'activa' && !intentado) {
		intentado = true;
		iniciar();
	}

	let estado = 'conectando';
	let texto = '';
	let chat = null;

	async function iniciar() {
		cargando = true;
		try {
			const p = await mensajesDM(cid);
			historial = Array.isArray(p?.datos) ? p.datos : [];
			conectar();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	function conectar() {
		chat?.cerrar();
		estado = 'conectando';
		chat = conectarDM(cid, {
			onMensaje: (m) => {
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
	}

	function enviar() {
		const cuerpo = texto.trim();
		if (!cuerpo) return;
		if (!chat?.enviar(cuerpo)) {
			error = 'conexión caída; reconectando…';
			conectar();
			return;
		}
		texto = '';
		error = '';
	}

	async function borrar(m) {
		try {
			await borrarMensajeDM(m.id);
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

<svelte:head><title>Mensaje directo · Universidad App</title></svelte:head>

<a href="/mensajes">← Mensajes</a>

{#if $sesion.estado !== 'activa'}
	<p>Iniciar sesión para ver la conversación.</p>
{:else}
	<div class="barra">
		<h1>Mensaje directo</h1>
		<span class="indicador {estado}">
			{estado === 'en-vivo' ? '● en vivo' : estado === 'conectando' ? '… conectando' : '○ caído — reintentando'}
		</span>
	</div>

	{#if cargando}<p>Cargando…</p>{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}

	<div class="historial">
		{#each historial || [] as m}
			<div class="fila {m.autor_id === $sesion.usuario?.sub ? 'propio' : ''}">
				<div class="burbuja">
					<span class="meta">{m.autor_nombre} · {hora(m.created_at)}</span>
					<p>{m.cuerpo}</p>
				</div>
				<button class="borrar" title="Borrar mensaje" on:click={() => borrar(m)}>✕</button>
			</div>
		{/each}
	</div>

	<form class="entrada" on:submit|preventDefault={enviar}>
		<input bind:value={texto} placeholder="Escribe un mensaje…" maxlength="2000" />
		<button type="submit" disabled={!texto.trim()}>Enviar</button>
	</form>
{/if}

<style>
	.barra { display: flex; justify-content: space-between; align-items: center; }
	.indicador { font-size: 0.85rem; }
	.indicador.en-vivo { color: #4ade80; }
	.indicador.conectando { color: #facc15; }
	.indicador.caido { color: #f87171; }
	.historial { display: flex; flex-direction: column; gap: 0.5rem; margin: 0.8rem 0; max-height: 60vh; overflow-y: auto; }
	.fila { display: flex; align-items: flex-start; gap: 0.4rem; }
	.fila.propio { flex-direction: row-reverse; }
	.burbuja { max-width: 80%; padding: 0.5rem 0.8rem; border-radius: 12px; background: #1e293b; }
	.fila.propio .burbuja { background: #0c4a6e; }
	.burbuja p { margin: 0.2rem 0 0; }
	.meta { color: #94a3b8; font-size: 0.75rem; }
	.borrar { background: none; border: 0; color: #64748b; cursor: pointer; font-size: 0.8rem; padding: 0.2rem; }
	.borrar:hover { color: #f87171; }
	.entrada { display: flex; gap: 0.5rem; position: sticky; bottom: 4rem; }
	.entrada input { flex: 1; padding: 0.6rem; border-radius: 8px; border: 1px solid #334155; background: #1e293b; color: #e2e8f0; }
	.entrada button { padding: 0.6rem 1.2rem; border: 0; border-radius: 8px; background: #0ea5e9; color: #082f49; font-weight: 700; cursor: pointer; }
	.error { color: #f87171; }
</style>
