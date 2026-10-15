<script>
	import { page } from '$app/stores';
	import { sesion } from '$lib/auth.js';
	import { goto } from '$app/navigation';
	import {
		avisos, publicarAviso, crearVotacion, votar, resultados,
		comentarios, comentar
	} from '$lib/api.js';

	const sid = $page.params.sid;
	$: if ($sesion.estado === 'anonimo') goto('/login');
	
	let lista = [];
	let cargando = true;
	let error = '';
	let intentado = false;

	$: if ($sesion.estado === 'activa' && !intentado) {
		intentado = true;
		cargar();
	}

	let mostrarCompositor = false;
	let tipoNuevo = 'aviso';
	let titulo = '';
	let cuerpo = '';
	let pregunta = '';
	let opcionesTexto = '';
	let multiple = false;
	let publicando = false;

	let resPorVotacion = {};
	let comentariosPorAviso = {};

	async function cargar() {
		cargando = true;
		error = '';
		try {
			const p = await avisos(sid);
			lista = Array.isArray(p?.datos) ? p.datos : [];
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	async function publicar() {
		publicando = true;
		error = '';
		try {
			if (tipoNuevo === 'aviso') {
				await publicarAviso(sid, titulo, cuerpo);
			} else {
				await crearVotacion(sid, {
					titulo,
					pregunta,
					opciones: opcionesTexto.split('\n').map((o) => o.trim()).filter(Boolean),
					multiple
				});
			}
			titulo = cuerpo = pregunta = opcionesTexto = '';
			multiple = false;
			mostrarCompositor = false;
			await cargar();
		} catch (e) {
			error = e.message;
		} finally {
			publicando = false;
		}
	}

	async function votarEn(aviso, idx) {
		error = '';
		try {
			await votar(aviso.votacion.id, [idx]);
			resPorVotacion[aviso.votacion.id] = await resultados(aviso.votacion.id);
			resPorVotacion = { ...resPorVotacion };
		} catch (e) {
			error = e.message;
		}
	}

	async function verResultados(aviso) {
		resPorVotacion[aviso.votacion.id] = await resultados(aviso.votacion.id);
		resPorVotacion = { ...resPorVotacion };
	}

	async function toggleComentarios(aviso) {
		const estado = comentariosPorAviso[aviso.id] || { abierto: false, lista: [], texto: '' };
		estado.abierto = !estado.abierto;
		comentariosPorAviso[aviso.id] = estado;
		comentariosPorAviso = { ...comentariosPorAviso };
		if (estado.abierto && estado.lista.length === 0) {
			const p = await comentarios(aviso.id);
			estado.lista = Array.isArray(p?.datos) ? p.datos : [];
			comentariosPorAviso = { ...comentariosPorAviso };
		}
	}

	async function enviarComentario(aviso) {
		const estado = comentariosPorAviso[aviso.id];
		await comentar(aviso.id, estado.texto.trim());
		estado.texto = '';
		const p = await comentarios(aviso.id);
		estado.lista = Array.isArray(p?.datos) ? p.datos : [];
		comentariosPorAviso = { ...comentariosPorAviso };
	}
</script>

<svelte:head><title>Sección · Universidad App</title></svelte:head>

<a href="/">← Mis secciones</a>

{#if $sesion.estado !== 'activa'}
	<p>Iniciar sesión para ver esta sección.</p>
{:else}
	<div class="barra">
		<h1>Novedades</h1>
		<button on:click={() => (mostrarCompositor = !mostrarCompositor)}>
			{mostrarCompositor ? 'Cancelar' : '+ Publicar'}
		</button>
	</div>

	{#if mostrarCompositor}
		<div class="compositor">
			<label>Tipo
				<select bind:value={tipoNuevo}>
					<option value="aviso">Aviso</option>
					<option value="votacion">Votación</option>
				</select>
			</label>
			<label>Título <input bind:value={titulo} required /></label>
			{#if tipoNuevo === 'aviso'}
				<label>Cuerpo <textarea bind:value={cuerpo} rows="3"></textarea></label>
			{:else}
				<label>Pregunta <input bind:value={pregunta} required /></label>
				<label>Opciones (una por línea, mínimo 2)
					<textarea bind:value={opcionesTexto} rows="3" required></textarea>
				</label>
				<label class="fila"><input type="checkbox" bind:checked={multiple} /> Permite elegir varias</label>
			{/if}
			{#if error}<p class="error" role="alert">{error}</p>{/if}
			<button on:click={publicar} disabled={publicando}>
				{publicando ? 'Publicando…' : 'Publicar'}
			</button>
		</div>
	{/if}

	{#if !mostrarCompositor && error}
		<p class="error" role="alert">{error}</p>
	{/if}

	{#if cargando}
		<p>Cargando…</p>
	{/if}

	{#each lista || [] as aviso}
		<article class="aviso">
			<h2>{aviso.titulo}</h2>
			<!-- en votaciones el cuerpo del aviso DUPLICA la pregunta: solo se muestra en avisos -->
			{#if aviso.tipo === 'aviso' && aviso.cuerpo}<p>{aviso.cuerpo}</p>{/if}

			{#if aviso.votacion}
				<div class="votacion">
					<p class="pregunta">{aviso.votacion.pregunta}</p>
					{#if resPorVotacion[aviso.votacion.id]}
						{@const res = resPorVotacion[aviso.votacion.id]}
						{#each res.conteo || [] as c}
							<div class="barra-result">
								<span>{c.opcion}</span>
								<div class="barra"><div class="relleno" style={`width: ${res.total_votos ? (c.votos / res.total_votos) * 100 : 0}%`}></div></div>
								<span>{c.votos}</span>
							</div>
						{/each}
						<p class="meta">
							{res.total_votos} {res.total_votos === 1 ? 'voto' : 'votos'}
							· {res.participacion} {res.participacion === 1 ? 'participante' : 'participantes'}
						</p>
					{:else if aviso.votacion.cerrada}
						<p class="meta">Votación cerrada.</p>
						<button on:click={() => verResultados(aviso)}>Ver resultados</button>
					{:else}
						{#each aviso.votacion.opciones || [] as op, i}
							<button class="opcion" on:click={() => votarEn(aviso, i)}>{op}</button>
						{/each}
						<button class="link" on:click={() => verResultados(aviso)}>Resultados</button>
					{/if}
				</div>
			{/if}

			<footer>
				<button class="link" on:click={() => toggleComentarios(aviso)}>Comentarios</button>
			</footer>

			{#if comentariosPorAviso[aviso.id]?.abierto}
				<div class="comentarios">
					{#each comentariosPorAviso[aviso.id].lista || [] as c}
						<p class="comentario">{c.cuerpo}</p>
					{/each}
					<form on:submit|preventDefault={() => enviarComentario(aviso)}>
						<input bind:value={comentariosPorAviso[aviso.id].texto} placeholder="Escribe un comentario…" required />
						<button type="submit">Enviar</button>
					</form>
				</div>
			{/if}
		</article>
	{/each}
{/if}

<style>
	.barra { display: flex; justify-content: space-between; align-items: center; }
	.compositor { background: #1e293b; padding: 1rem; border-radius: 10px; margin-bottom: 1rem; }
	label { display: block; margin: 0.6rem 0; }
	input, textarea, select {
		display: block; width: 100%; padding: 0.5rem; margin-top: 0.25rem;
		border-radius: 8px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0;
	}
	label.fila { display: flex; gap: 0.5rem; align-items: center; }
	label.fila input { width: auto; }
	.aviso { background: #1e293b; border-radius: 10px; padding: 1rem; margin: 0.8rem 0; }
	.pregunta { font-weight: 600; }
	.opcion { display: block; width: 100%; margin: 0.3rem 0; padding: 0.6rem; text-align: left; border-radius: 8px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0; cursor: pointer; }
	.barra-result { display: grid; grid-template-columns: auto 1fr auto; gap: 0.5rem; align-items: center; margin: 0.3rem 0; }
	.barra { background: #0f172a; border-radius: 6px; height: 12px; }
	.relleno { background: #0ea5e9; height: 100%; border-radius: 6px; }
	.meta { color: #94a3b8; font-size: 0.85rem; }
	.link { background: none; border: 0; color: #7dd3fc; cursor: pointer; font: inherit; padding: 0; }
	.comentarios { border-top: 1px solid #334155; margin-top: 0.6rem; padding-top: 0.6rem; }
	.comentario { background: #0f172a; border-radius: 8px; padding: 0.5rem 0.8rem; margin: 0.4rem 0; }
	.comentarios form { display: flex; gap: 0.5rem; margin-top: 0.5rem; }
	.comentarios input { flex: 1; }
	.error { color: #f87171; }
</style>
