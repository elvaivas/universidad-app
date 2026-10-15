<script>
	import { login, login2fa, cambiarPassword } from '$lib/api.js';
	import { iniciarSesion } from '$lib/auth.js';
	import { goto } from '$app/navigation';

	let paso = 'credenciales';
	let email = '';
	let password = '';
	let mostrar = false;
	let codigo = '';
	let pre2faToken = '';
	let actual = '';
	let nueva = '';
	let mostrarNueva = false;
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
		<label>Correo electrónico
			<input type="email" bind:value={email} autocomplete="username" />
		</label>
		<label>Contraseña
			<span class="campo">
				{#if mostrar}
					<input type="text" bind:value={password} autocomplete="current-password" />
				{:else}
					<input type="password" bind:value={password} autocomplete="current-password" />
				{/if}
				<button type="button" class="ojito" on:click={() => (mostrar = !mostrar)}>
					{mostrar ? 'Ocultar' : 'Mostrar'}
				</button>
			</span>
		</label>
		<button type="submit" disabled={cargando}>{cargando ? 'Entrando…' : 'Entrar'}</button>
	</form>
{:else if paso === 'codigo2fa'}
	<h1>Verificación en dos pasos</h1>
	<p>Ingresa el código de 6 dígitos de tu aplicación autenticadora.</p>
	<form on:submit|preventDefault={enviarCodigo}>
		<label>Código
			<input inputmode="numeric" maxlength="6" bind:value={codigo} />
		</label>
		<button type="submit" disabled={cargando}>{cargando ? 'Verificando…' : 'Verificar'}</button>
	</form>
{:else}
	<h1>Definir una contraseña nueva</h1>
	<p>Por seguridad, la contraseña inicial debe sustituirse por una propia (mínimo 10 caracteres).</p>
	<form on:submit|preventDefault={enviarCambio}>
		<label>Contraseña actual
			<input type="password" bind:value={actual} autocomplete="current-password" />
		</label>
		<label>Contraseña nueva
			<span class="campo">
				{#if mostrarNueva}
					<input type="text" bind:value={nueva} autocomplete="new-password" />
				{:else}
					<input type="password" bind:value={nueva} autocomplete="new-password" />
				{/if}
				<button type="button" class="ojito" on:click={() => (mostrarNueva = !mostrarNueva)}>
					{mostrarNueva ? 'Ocultar' : 'Mostrar'}
				</button>
			</span>
		</label>
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
	.campo { position: relative; display: block; margin-top: 0.25rem; }
	.campo input { margin-top: 0; padding-right: 5rem; }
	.ojito {
		position: absolute; right: 0.4rem; top: 50%; transform: translateY(-50%);
		background: none; border: 0; color: #7dd3fc; cursor: pointer; font-size: 0.85rem;
		padding: 0.3rem 0.4rem;
	}
	button[type='submit'] {
		padding: 0.7rem 1.2rem; border: 0; border-radius: 8px;
		background: #0ea5e9; color: #082f49; font-weight: 700; cursor: pointer;
	}
	button:disabled { opacity: 0.6; cursor: default; }
	.error { color: #f87171; }
</style>
