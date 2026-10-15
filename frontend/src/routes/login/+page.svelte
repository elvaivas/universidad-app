<script>
	import { login, login2fa, cambiarPassword } from '$lib/api.js';
	import { iniciarSesion } from '$lib/auth.js';
	import { goto } from '$app/navigation';

	let paso = 'credenciales'; // credenciales | codigo2fa | cambioPassword
	let email = '';
	let password = '';
	let codigo = '';
	let pre2faToken = '';
	let actual = '';
	let nueva = '';
	let error = '';
	let cargando = false;

	async function entrar() {
		await iniciarSesion();
		goto('/');
	}

	async function enviarCredenciales() {
		cargando = true;
		error = '';
		try {
			const r = await login(email.trim(), password);
			if (r.requiere2fa) {
				pre2faToken = r.pre2faToken;
				paso = 'codigo2fa';
			} else if (r.forcePasswordChange) {
				paso = 'cambioPassword';
			} else {
				await entrar();
			}
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	async function enviarCodigo() {
		cargando = true;
		error = '';
		try {
			const r = await login2fa(pre2faToken, codigo.trim());
			if (r.forcePasswordChange) {
				paso = 'cambioPassword';
			} else {
				await entrar();
			}
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}

	async function enviarCambio() {
		cargando = true;
		error = '';
		try {
			await cambiarPassword(actual, nueva);
			await entrar();
		} catch (e) {
			error = e.message;
		} finally {
			cargando = false;
		}
	}
</script>

<svelte:head><title>Entrar · Universidad App</title></svelte:head>

{#if paso === 'credenciales'}
	<h1>Entrar</h1>
	<form on:submit|preventDefault={enviarCredenciales}>
		<label>Email <input type="email" bind:value={email} required autocomplete="username" /></label>
		<label>Contraseña <input type="password" bind:value={password} required autocomplete="current-password" /></label>
		<button type="submit" disabled={cargando}>{cargando ? 'Entrando…' : 'Entrar'}</button>
	</form>
{:else if paso === 'codigo2fa'}
	<h1>Segundo factor</h1>
	<p>Ingresa el código de 6 dígitos de tu app autenticadora.</p>
	<form on:submit|preventDefault={enviarCodigo}>
		<label>Código <input inputmode="numeric" pattern="[0-9]{6}" maxlength="6" bind:value={codigo} required /></label>
		<button type="submit" disabled={cargando}>{cargando ? 'Verificando…' : 'Verificar'}</button>
	</form>
{:else}
	<h1>Cambia tu contraseña</h1>
	<p>Por seguridad debes definir una contraseña nueva (mínimo 10 caracteres).</p>
	<form on:submit|preventDefault={enviarCambio}>
		<label>Contraseña actual <input type="password" bind:value={actual} required autocomplete="current-password" /></label>
		<label>Contraseña nueva <input type="password" bind:value={nueva} required minlength="10" autocomplete="new-password" /></label>
		<button type="submit" disabled={cargando}>{cargando ? 'Guardando…' : 'Guardar y continuar'}</button>
	</form>
{/if}

{#if error}<p class="error" role="alert">{error}</p>{/if}

<style>
	label { display: block; margin: 0.8rem 0; }
	input {
		display: block; width: 100%; padding: 0.6rem; margin-top: 0.25rem;
		border-radius: 8px; border: 1px solid #334155; background: #1e293b; color: #e2e8f0;
	}
	button {
		padding: 0.7rem 1.2rem; border: 0; border-radius: 8px;
		background: #0ea5e9; color: #082f49; font-weight: 700; cursor: pointer;
	}
	button:disabled { opacity: 0.6; cursor: default; }
	.error { color: #f87171; }
</style>
