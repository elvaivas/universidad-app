<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sesion, salir } from '$lib/auth.js';
	import { seguridad, activar2FA, confirmar2FA, desactivar2FA } from '$lib/api.js';

	let est = null;
	let cargando = true;
	let error = '';

	let pendiente = null;
	let codigo = '';
	let activando = false;

	let confirmandoBaja = false;
	let codigoBaja = '';

	// si la sesión muere (logout o expiración), volver al login
	$: if ($sesion.estado === 'anonimo') goto('/login');

	async function cargar() {
		cargando = true;
		error = '';
		try {
			est = await seguridad();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	onMount(cargar);

	async function activar() {
		activando = true;
		error = '';
		try {
			pendiente = await activar2FA();
		} catch (e) {
			error = e.message;
		} finally {
			activando = false;
		}
	}

	async function confirmar() {
		activando = true;
		error = '';
		try {
			await confirmar2FA(codigo.trim());
			pendiente = null;
			codigo = '';
			await cargar();
		} catch (e) {
			error = e.message;
		} finally {
			activando = false;
		}
	}

	async function desactivar() {
		activando = true;
		error = '';
		try {
			await desactivar2FA(codigoBaja.trim());
			confirmandoBaja = false;
			codigoBaja = '';
			await cargar();
		} catch (e) {
			error = e.message;
		} finally {
			activando = false;
		}
	}
</script>

<svelte:head><title>Perfil · Universidad App</title></svelte:head>

{#if $sesion.estado !== 'activa'}
	<p>Iniciar sesión para ver el perfil.</p>
{:else if cargando}
	<p>Cargando…</p>
{:else if error}
	<p class="error" role="alert">{error}</p>
{:else if est}
	<section class="tarjeta">
		<h1>{est.nombre}</h1>
		<p class="dato">{est.email}</p>
		<p class="dato">Rol: <strong>{est.rol_base}</strong>{est.es_mantenedor ? ' · mantenedor' : ''}</p>
		<p class="dato">Secciones: {$sesion.usuario?.seccion_ids?.length ?? 0}</p>
	</section>

	<section class="tarjeta">
		<h2>Verificación en dos pasos (2FA)</h2>
		{#if pendiente}
			<p>Escanea este código QR con tu aplicación autenticadora (Google Authenticator, Aegis…) e ingresa el código generado para confirmar.</p>
			<img class="qr" src={`data:image/png;base64,${pendiente.qr_png_base64}`} alt="Código QR para configurar 2FA" />
			<details>
				<summary>¿No puedes escanear? Usa el código manual</summary>
				<code>{pendiente.secreto}</code>
			</details>
			<form on:submit|preventDefault={confirmar}>
				<label>Código de 6 dígitos
					<input inputmode="numeric" maxlength="6" bind:value={codigo} />
				</label>
				<button type="submit" disabled={activando}>{activando ? 'Confirmando…' : 'Confirmar activación'}</button>
				<button type="button" class="secundario" on:click={() => (pendiente = null)}>Cancelar</button>
			</form>
		{:else if est.totp_activado}
			<p class="ok">El segundo factor está <strong>activo</strong>. Se pedirá un código adicional al iniciar sesión.</p>
			{#if confirmandoBaja}
				<form on:submit|preventDefault={desactivar}>
					<label>Para desactivar, ingresa un código vigente
						<input inputmode="numeric" maxlength="6" bind:value={codigoBaja} />
					</label>
					<button type="submit" disabled={activando}>{activando ? 'Desactivando…' : 'Confirmar desactivación'}</button>
					<button type="button" class="secundario" on:click={() => (confirmandoBaja = false)}>Cancelar</button>
				</form>
			{:else}
				<button class="peligro" on:click={() => (confirmandoBaja = true)}>Desactivar 2FA</button>
			{/if}
		{:else}
			<p>Tu cuenta depende solo de la contraseña. Activa el segundo factor para protegerla.</p>
			<button on:click={activar} disabled={activando}>{activando ? 'Generando…' : 'Activar 2FA'}</button>
		{/if}
	</section>

	<button class="salir" on:click={salir}>Cerrar sesión</button>
{/if}

<style>
	.tarjeta { background: #1e293b; border-radius: 10px; padding: 1rem; margin: 0.8rem 0; }
	h1 { margin: 0 0 0.3rem; }
	h2 { margin-top: 0; }
	.dato { color: #94a3b8; margin: 0.2rem 0; }
	.qr { display: block; margin: 0.8rem auto; border-radius: 10px; background: #fff; padding: 8px; }
	code { display: inline-block; background: #0f172a; padding: 0.4rem 0.6rem; border-radius: 6px; margin: 0.4rem 0; }
	label { display: block; margin: 0.8rem 0; }
	input {
		display: block; width: 100%; padding: 0.6rem; margin-top: 0.25rem;
		border-radius: 8px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0;
	}
	button {
		padding: 0.7rem 1.2rem; border: 0; border-radius: 8px; cursor: pointer;
		background: #0ea5e9; color: #082f49; font-weight: 700; margin-right: 0.5rem;
	}
	button:disabled { opacity: 0.6; cursor: default; }
	.secundario { background: #334155; color: #e2e8f0; }
	.peligro { background: #7f1d1d; color: #fecaca; }
	.salir { display: block; width: 100%; margin-top: 1rem; background: #1e293b; color: #f87171; }
	.ok { color: #4ade80; }
	.error { color: #f87171; }
</style>
