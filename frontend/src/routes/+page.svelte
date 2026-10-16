<script>
	import { sesion } from '$lib/auth.js';
	import {
		inicio, misSecciones, crearAnuncioGeneral, actualizarAnuncioGeneral,
		eliminarAnuncioGeneral, marcarNotificacionLeida, panelNotificaciones
	} from '$lib/api.js';

	let datos = null;
	let secciones = [];
	let cargando = true;
	let error = '';
	let intentado = false;

	$: if ($sesion.estado === 'activa' && !intentado) {
		intentado = true;
		cargar();
	}

	async function cargar() {
		cargando = true;
		error = '';
		try {
			datos = await inicio();
			secciones = await misSecciones();
			const p = await panelNotificaciones();
			for (const n of p.no_leidas || []) {
				if (n.tipo === 'anuncio_general' || n.tipo === 'aviso_importante') {
					await marcarNotificacionLeida({ id: n.id });
				}
			}
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	// publicador / editor de anuncios generales (mantenedor)
	let mostrarPublicador = false;
	let editando = null; // anuncio en edición (null = crear nuevo)
	let titulo = '';
	let cuerpo = '';
	let publicando = false;

	function abrirCrear() {
		editando = null;
		titulo = cuerpo = '';
		mostrarPublicador = true;
	}

	function abrirEditar(a) {
		editando = a;
		titulo = a.titulo;
		cuerpo = a.cuerpo;
		mostrarPublicador = true;
	}

	async function guardar() {
		publicando = true;
		error = '';
		try {
			if (editando) {
				await actualizarAnuncioGeneral(editando.id, titulo, cuerpo);
			} else {
				await crearAnuncioGeneral(titulo, cuerpo);
			}
			titulo = cuerpo = '';
			mostrarPublicador = false;
			editando = null;
			await cargar();
		} catch (e) {
			error = e.message;
		} finally {
			publicando = false;
		}
	}

	async function borrar(a) {
		if (!confirm(`¿Eliminar el anuncio "${a.titulo}"?`)) return;
		error = '';
		try {
			await eliminarAnuncioGeneral(a.id);
			await cargar();
		} catch (e) {
			error = e.message;
		}
	}
</script>

<svelte:head><title>Inicio · Universidad App</title></svelte:head>

{#if $sesion.estado === 'cargando'}
	<p>Cargando…</p>
{:else if $sesion.estado === 'activa'}
	{#if cargando}
		<p>Cargando…</p>
	{:else if error}
		<p class="error" role="alert">{error}</p>
	{:else}
		<section class="tarjeta universidad">
			<div class="cabecera-seccion">
				<h1>Universidad</h1>
				{#if $sesion.usuario?.es_mantenedor}
					<button class="mini" on:click={abrirCrear}>+ Anuncio</button>
				{/if}
			</div>
			{#if mostrarPublicador}
				<div class="publicador">
					<h3>{editando ? 'Editar anuncio' : 'Nuevo anuncio para toda la universidad'}</h3>
					<label>Título <input bind:value={titulo} /></label>
					<label>Cuerpo <textarea bind:value={cuerpo} rows="2"></textarea></label>
					<button on:click={guardar} disabled={publicando}>
						{publicando ? 'Guardando…' : editando ? 'Guardar cambios' : 'Publicar a todos'}
					</button>
					<button class="cancelar" on:click={() => (mostrarPublicador = false)}>Cancelar</button>
				</div>
			{/if}
			{#if datos.universidad.length === 0}
				<p class="meta">Sin anuncios institucionales por ahora.</p>
			{:else}
				{#each datos.universidad as a}
					<article class="aviso-universidad">
						<div class="fila-anuncio">
							<div class="contenido">
								<strong>{a.titulo}</strong>
								{#if a.cuerpo}<p>{a.cuerpo}</p>{/if}
							</div>
							{#if $sesion.usuario?.es_mantenedor}
								<div class="acciones">
									<button class="mini" on:click={() => abrirEditar(a)}>Editar</button>
									<button class="mini peligro" on:click={() => borrar(a)}>Eliminar</button>
								</div>
							{/if}
						</div>
					</article>
				{/each}
			{/if}
		</section>

		{#if datos.importantes.length > 0}
			<section class="tarjeta">
				<h2>Importantes de tus secciones</h2>
				{#each datos.importantes as a}
					<article class="aviso-importante">
						<a href={`/secciones/${a.seccion_id}`}><strong>{a.titulo}</strong></a>
						{#if a.cuerpo}<p>{a.cuerpo}</p>{/if}
					</article>
				{/each}
			</section>
		{/if}

		<section class="tarjeta">
			<h2>Mis secciones</h2>
			{#if secciones.length === 0}
				<p class="meta">No estás inscrito en ninguna sección todavía.</p>
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
		</section>
	{/if}
{:else}
	<h1>Bienvenido</h1>
	<p><a href="/login">Iniciar sesión</a></p>
{/if}

<style>
	.tarjeta { background: #1e293b; border-radius: 10px; padding: 1rem; margin: 0.8rem 0; }
	.universidad { border: 1px solid #0ea5e9; }
	.cabecera-seccion { display: flex; justify-content: space-between; align-items: center; }
	h1 { margin: 0; font-size: 1.2rem; color: #7dd3fc; }
	h2 { margin-top: 0; }
	h3 { margin-top: 0; font-size: 0.95rem; }
	.mini { background: #0c4a6e; color: #7dd3fc; border: 0; border-radius: 8px; padding: 0.3rem 0.8rem; cursor: pointer; font-weight: 600; font-size: 0.8rem; }
	.mini.peligro { background: #7f1d1d; color: #fecaca; }
	.aviso-universidad { border-top: 1px solid #334155; padding: 0.6rem 0; }
	.aviso-universidad:first-of-type { border-top: 0; }
	.fila-anuncio { display: flex; justify-content: space-between; gap: 0.6rem; align-items: flex-start; }
	.contenido { flex: 1; }
	.contenido p { margin: 0.2rem 0 0; color: #cbd5e1; }
	.acciones { display: flex; gap: 0.3rem; flex-shrink: 0; }
	.aviso-importante { border-left: 3px solid #facc15; padding: 0.3rem 0.6rem; margin: 0.4rem 0; background: #0f172a; border-radius: 6px; }
	.aviso-importante a { color: #e2e8f0; text-decoration: none; }
	.aviso-importante p { margin: 0.2rem 0 0; color: #cbd5e1; }
	.lista { list-style: none; padding: 0; }
	.lista li { margin: 0.5rem 0; }
	.lista a { display: block; padding: 0.8rem 1rem; border-radius: 10px; background: #0f172a; text-decoration: none; color: #e2e8f0; }
	.lista a span { display: block; color: #94a3b8; font-size: 0.9rem; margin-top: 0.2rem; }
	.publicador { background: #0f172a; padding: 0.8rem; border-radius: 8px; margin: 0.6rem 0; }
	.publicador label { display: block; margin: 0.5rem 0; }
	.publicador input, .publicador textarea {
		display: block; width: 100%; padding: 0.5rem; margin-top: 0.2rem;
		border-radius: 8px; border: 1px solid #334155; background: #1e293b; color: #e2e8f0;
	}
	.publicador button {
		padding: 0.5rem 1rem; border: 0; border-radius: 8px;
		background: #0ea5e9; color: #082f49; font-weight: 700; cursor: pointer;
	}
	.publicador .cancelar { background: #334155; color: #e2e8f0; margin-left: 0.4rem; }
	.meta { color: #94a3b8; }
	.error { color: #f87171; }
</style>
