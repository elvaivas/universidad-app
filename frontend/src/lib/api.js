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

	if (resp.status === 401 && !ruta.startsWith('/api/auth/')) {
		// un solo reintento por llamada: tras refrescar, si sigue 401 es
		// token inválido de verdad — no hay bucle infinito de refresh.
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
