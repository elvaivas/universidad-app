<script>
	import { goto } from '$app/navigation';
	import { sesion } from '$lib/auth.js';
	import {
		solicitudesRecibidas, aceptarSolicitud, rechazarSolicitud,
		crearSolicitudDM, conversaciones
	} from '$lib/api.js';

	let solicitudes = [];
	let convos = [];
	let cargando = true;
	let error = '';
	let intentado = false;

	$: if ($sesion.estado === 'anonimo') goto('/login');
	$: if ($sesion.estado === 'activa' && !intentado) {
		intentado = true;
		cargar();
	}

	let email = '';
	let mensaje = '';
	let enviando = false;
	let confirmacion = '';

	async function cargar() {
		cargando = true;
		error = '';
		try {
			solicitudes = await solicitudesRecibidas();
			convos = await conversaciones();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	async function aceptar(s) {
		error = '';
		try {
			const r = await aceptarSolicitud(s.id);
			goto(`/mensajes/${r.conversacion_id}`);
		} catch (e) {
			error = e.message;
		}
	}

	async function rechazar(s) {
		error = '';
		try {
			await rechazarSolicitud(s.id);
			await cargar();
		} catch (e) {
			error = e.message;
		}
	}

	async function enviarSolicitud() {
		enviando = true;
		error = '';
		confirmacion = '';
		try {
			await crearSolicitudDM(email.trim(), mensaje.trim());
			confirmacion = 'Solicitud enviada. La persona la verá cuando entre a sus mensajes.';
			email = mensaje = '';
		} catch (e) {
			error = e.message;
		} finally {
			enviando = false;
		}
	}
</script>

<svelte:head><title>Mensajes · Universidad App</title></svelte:head>

{#if $sesion.estado !== 'activa'}
	<p>Iniciar sesión para ver los mensajes.</p>
{:else if cargando}
	<p>Cargando…</p>
{:else}
	<h1>Mensajes</h1>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	{#if confirmacion}<p class="ok">{confirmacion}</p>{/if}

	{#if solicitudes.length > 0}
		<section class="tarjeta">
			<h2>Solicitudes ({solicitudes.length})</h2>
			{#each solicitudes as s}
				<div class="solicitud">
					<p><strong>{s.solicitante_nombre}</strong> <span class="meta">{s.solicitante_email}</span></p>
					{#if s.mensaje}<p class="intencion">“{s.mensaje}”</p>{/if}
					<button on:click={() => aceptar(s)}>Aceptar</button>
					<button class="secundario" on:click={() => rechazar(s)}>Rechazar</button>
				</div>
			{/each}
		</section>
	{/if}

	<section class="tarjeta">
		<h2>Conversaciones</h2>
		{#if convos.length === 0}
			<p class="meta">Sin conversaciones todavía.</p>
		{:else}
			<ul class="lista">
				{#each convos as c}
					<li><a href={`/mensajes/${c.id}`}><strong>{c.otro_nombre}</strong> <span class="meta">{c.otro_email}</span></a></li>
				{/each}
			</ul>
		{/if}
	</section>

	<section class="tarjeta">
		<h2>Nueva solicitud de mensaje directo</h2>
		<p class="meta">La persona verá tu mensaje y decidirá si acepta conversar. Según tu rol, solo puedes contactar a personas de tus mismas secciones.</p>
		<form on:submit|preventDefault={enviarSolicitud}>
			<label>Correo de la persona
				<input type="email" bind:value={email} required />
			</label>
			<label>Mensaje de presentación
				<textarea bind:value={mensaje} rows="2" maxlength="500" placeholder="Ej: Hola, soy de tu sección de Programación I y tengo una duda sobre la tarea."></textarea>
			</label>
			<button type="submit" disabled={enviando}>{enviando ? 'Enviando…' : 'Enviar solicitud'}</button>
		</form>
	</section>
{/if}

<style>
	h1 { margin-bottom: 0.4rem; }
	.tarjeta { background: #1e293b; border-radius: 10px; padding: 1rem; margin: 0.8rem 0; }
	h2 { margin-top: 0; }
	.meta { color: #94a3b8; font-size: 0.85rem; }
	.intencion { background: #0f172a; border-left: 3px solid #0ea5e9; padding: 0.4rem 0.6rem; border-radius: 6px; font-style: italic; }
	.solicitud { border-top: 1px solid #334155; padding: 0.6rem 0; }
	.solicitud:first-of-type { border-top: 0; }
	label { display: block; margin: 0.6rem 0; }
	input, textarea {
		display: block; width: 100%; padding: 0.6rem; margin-top: 0.25rem;
		border-radius: 8px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0;
	}
	button {
		padding: 0.5rem 1rem; border: 0; border-radius: 8px; cursor: pointer;
		background: #0ea5e9; color: #082f49; font-weight: 700;
	}
	.secundario { background: #334155; color: #e2e8f0; margin-left: 0.4rem; }
	.lista { list-style: none; padding: 0; }
	.lista a { display: block; padding: 0.6rem; border-radius: 8px; background: #0f172a; margin: 0.3rem 0; text-decoration: none; color: #e2e8f0; }
	.error { color: #f87171; }
	.ok { color: #4ade80; }
</style>
