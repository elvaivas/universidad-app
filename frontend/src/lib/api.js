// Cliente API de la PWA.
// - access token en memoria (nunca en localStorage: XSS no debe poder robarlo).
// - refresh token solo en cookie httpOnly; el body se usa como respaldo.
// - refresh en CADENA ÚNICA: si 5 llamadas reciben 401 a la vez, solo UNA
//   ejecuta /refresh y las demás esperan su resultado.
// - un solo reintento por llamada; si el refresh falla → sesión expirada.

let accessToken = null;
let refreshToken = null; // respaldo en memoria para el body (la cookie manda)
let refreshPromise = null;
let onSesionExpirada = () => {};

export function configurarCliente({ onUnauthorized } = {}) {
	if (onUnauthorized) onSesionExpirada = onUnauthorized;
}

export function fijarTokens({ access_token, refresh_token }) {
	accessToken = access_token ?? accessToken;
	refreshToken = refresh_token ?? refreshToken;
}

export function limpiarTokens() {
	accessToken = null;
	refreshToken = null;
}

async function llamarRefresh() {
	const resp = await fetch('/api/auth/refresh', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		credentials: 'include', // cookie httpOnly
		body: JSON.stringify({ refresh_token: refreshToken ?? '' })
	});
	if (!resp.ok) return false;
	const { data } = await resp.json();
	fijarTokens(data);
	return true;
}

// refrescarYSeguir: cadena única. Todas las llamadas concurrentes comparten
// la misma promesa; cuando resuelve, todas reintentan con el token nuevo.
async function refrescarYSeguir() {
	if (!refreshPromise) {
		refreshPromise = llamarRefresh().finally(() => {
			refreshPromise = null;
		});
	}
	return refreshPromise;
}

export async function api(ruta, { method = 'GET', body, _reintentado = false } = {}) {
	const headers = { 'Content-Type': 'application/json' };
	if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`;
	const resp = await fetch(ruta, {
		method,
		headers,
		credentials: 'include',
		body: body !== undefined ? JSON.stringify(body) : undefined
	});

	// Solo las rutas de PRE-autenticación se saltan el refresh; /me SÍ debe
	// dispararlo — es la llamada que recupera la sesión al recargar la página.
	const sinRefresh = [
		'/api/auth/login',
		'/api/auth/refresh',
		'/api/auth/2fa/login',
		'/api/auth/logout'
	];
	if (resp.status === 401 && !sinRefresh.includes(ruta)) {
		if (_reintentado) {
			onSesionExpirada();
			throw Object.assign(new Error('sesión expirada'), { status: 401 });
		}
		const ok = await refrescarYSeguir();
		if (ok) return api(ruta, { method, body, _reintentado: true });
		onSesionExpirada();
		throw Object.assign(new Error('sesión expirada'), { status: 401 });
	}

	const datos = await resp.json().catch(() => ({}));
	if (!resp.ok) {
		throw Object.assign(new Error(datos.error || `error ${resp.status}`), {
			status: resp.status,
			datos
		});
	}
	return datos;
}

// --- endpoints de auth ---

export async function login(email, password) {
	const { data } = await api('/api/auth/login', { method: 'POST', body: { email, password } });
	if (data.requiere_2fa) return { requiere2fa: true, pre2faToken: data.pre_2fa_token };
	fijarTokens(data);
	return { requiere2fa: false, forcePasswordChange: !!data.force_password_change };
}

export async function login2fa(pre2faToken, codigo) {
	const { data } = await api('/api/auth/2fa/login', {
		method: 'POST',
		body: { pre_2fa_token: pre2faToken, codigo }
	});
	fijarTokens(data);
	return { forcePasswordChange: !!data.force_password_change };
}

export async function cambiarPassword(passwordActual, passwordNueva) {
	return api('/api/auth/password', {
		method: 'POST',
		body: { password_actual: passwordActual, password_nueva: passwordNueva }
	});
}

export async function me() {
	const { data } = await api('/api/auth/me');
	return data;
}

export async function logout() {
	try {
		await api('/api/auth/logout', { method: 'POST', body: {} });
	} finally {
		limpiarTokens();
	}
}

// --- feed y secciones ---

export async function misSecciones() {
	const { data } = await api('/api/me/secciones');
	return data;
}

export async function avisos(seccionID, { limit = 20, offset = 0 } = {}) {
	const { data } = await api(`/api/secciones/${seccionID}/avisos?limit=${limit}&offset=${offset}`);
	return data;
}

export async function publicarAviso(seccionID, titulo, cuerpo) {
	const { data } = await api(`/api/secciones/${seccionID}/avisos`, {
		method: 'POST',
		body: { titulo, cuerpo }
	});
	return data;
}

export async function crearVotacion(seccionID, { titulo, pregunta, opciones, multiple = false }) {
	const { data } = await api(`/api/secciones/${seccionID}/votaciones`, {
		method: 'POST',
		body: { titulo, pregunta, opciones, multiple }
	});
	return data;
}

export async function votar(votacionID, opciones) {
	const { data } = await api(`/api/votaciones/${votacionID}/votar`, {
		method: 'POST',
		body: { opciones }
	});
	return data;
}

export async function resultados(votacionID) {
	const { data } = await api(`/api/votaciones/${votacionID}/resultados`);
	return data;
}

export async function comentarios(avisoID) {
	const { data } = await api(`/api/avisos/${avisoID}/comentarios?limit=50`);
	return data;
}

export async function comentar(avisoID, cuerpo) {
	const { data } = await api(`/api/avisos/${avisoID}/comentarios`, {
		method: 'POST',
		body: { cuerpo }
	});
	return data;
}

// --- 2FA y perfil (Bloque 11a) ---

export async function seguridad() {
	const { data } = await api('/api/auth/seguridad');
	return data;
}

export async function activar2FA() {
	const { data } = await api('/api/auth/2fa/activar', { method: 'POST', body: {} });
	return data; // { secreto, otpauth_url, qr_png_base64 }
}

export async function confirmar2FA(codigo) {
	const { data } = await api('/api/auth/2fa/confirmar', { method: 'POST', body: { codigo } });
	return data;
}

export async function desactivar2FA(codigo) {
	const { data } = await api('/api/auth/2fa/desactivar', { method: 'POST', body: { codigo } });
	return data;
}

// --- chat en tiempo real (Bloque 11b) ---

export async function mensajes(seccionID, { limit = 50, offset = 0 } = {}) {
	const { data } = await api(
		`/api/secciones/${seccionID}/mensajes?limit=${limit}&offset=${offset}`
	);
	return data;
}

export async function borrarMensaje(mensajeID) {
	const { data } = await api(`/api/mensajes/${mensajeID}`, { method: 'DELETE' });
	return data;
}

// conectarChat abre el WebSocket autenticado (?token=). El navegador no puede
// enviar headers en el upgrade WS, así que el access token viaja en la URL:
// dura solo 15 min y la membresía se re-verifica al conectar.
export function conectarChat(seccionID, { onMensaje, onBorrado, onError, onEstado } = {}) {
	const proto = location.protocol === 'https:' ? 'wss' : 'ws';
	const ws = new WebSocket(`${proto}://${location.host}/ws/secciones/${seccionID}?token=${encodeURIComponent(accessToken ?? '')}`);

	ws.onopen = () => onEstado?.('en-vivo');
	ws.onclose = () => onEstado?.('caido');
	ws.onerror = () => onEstado?.('caido');
	ws.onmessage = (e) => {
		let ev;
		try {
			ev = JSON.parse(e.data);
		} catch {
			return;
		}
		if (ev.tipo === 'mensaje') onMensaje?.(ev.data);
		else if (ev.tipo === 'borrado') onBorrado?.(ev.data);
		else if (ev.tipo === 'error') onError?.(ev.data?.error || 'error de chat');
	};

	return {
		enviar(cuerpo) {
			if (ws.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify({ tipo: 'mensaje', cuerpo }));
				return true;
			}
			return false;
		},
		cerrar() {
			ws.close();
		}
	};
}
