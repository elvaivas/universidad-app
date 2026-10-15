<svelte:head><title>Entrar · Universidad App</title></svelte:head>

<h1>Entrar</h1>
<p>La pantalla de login completa llega en la parte 9b del bloque (cliente API + 2FA + cambio de contraseña).</p>

<form on:submit|preventDefault>
	<label>Email <input type="email" required /></label>
	<label>Contraseña <input type="password" required /></label>
	<button type="submit" disabled>Entrar (próximamente)</button>
</form>
