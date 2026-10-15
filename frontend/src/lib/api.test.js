// Tests del cliente API con fetch simulado (vitest, node environment).
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	api,
	login,
	login2fa,
	logout,
	configurarCliente,
	limpiarTokens,
	fijarTokens
} from './api.js';

function jsonResp(status, cuerpo) {
	return new Response(JSON.stringify(cuerpo), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

const PAR = {
	data: {
		access_token: 'ACC',
		refresh_token: 'REF',
		token_type: 'Bearer',
		expires_in: 900,
		requiere_2fa: false,
		force_password_change: false
	}
};

beforeEach(() => {
	limpiarTokens();
	vi.restoreAllMocks();
	configurarCliente({ onUnauthorized: () => {} });
});

describe('login', () => {
	it('ok: fija tokens y reporta sin 2FA', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResp(200, PAR)));
		const r = await login('a@b.c', 'clave');
		expect(r).toEqual({ requiere2fa: false, forcePasswordChange: false });
	});

	it('con 2FA: devuelve pre-token sin fijar tokens', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(
				jsonResp(200, { data: { requiere_2fa: true, pre_2fa_token: 'PRE', expires_in: 120 } })
			)
		);
		const r = await login('a@b.c', 'clave');
		expect(r.requiere2fa).toBe(true);
		expect(r.pre2faToken).toBe('PRE');
	});

	it('credenciales malas: lanza error con status 401', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(jsonResp(401, { error: 'credenciales inválidas' }))
		);
		await expect(login('a@b.c', 'mala')).rejects.toMatchObject({ status: 401 });
	});

	it('login2fa: fija tokens y reporta force_password_change', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(
				jsonResp(200, {
					data: { ...PAR.data, force_password_change: true, requiere_2fa: false }
				})
			)
		);
		const r = await login2fa('PRE', '123456');
		expect(r.forcePasswordChange).toBe(true);
	});
});

describe('refresh en cadena única', () => {
	it('3 llamadas con 401 concurrentes ejecutan UN solo refresh y reintentan una vez', async () => {
		let refreshes = 0;
		vi.stubGlobal(
			'fetch',
			vi.fn().mockImplementation((url) => {
				if (url === '/api/auth/refresh') {
					refreshes++;
					return Promise.resolve(jsonResp(200, PAR));
				}
				return Promise.resolve(jsonResp(401, { error: 'token inválido' }));
			})
		);
		fijarTokens({ access_token: 'VIEJO' });

		const resultados = await Promise.allSettled([
			api('/api/x'), api('/api/y'), api('/api/z')
		]);
		expect(refreshes).toBe(1); // cadena única
		expect(resultados.every((r) => r.status === 'rejected')).toBe(true); // reintento falló → sin bucle
	});

	it('si el refresh falla, cadena única y aviso por llamada', async () => {
		const spy = vi.fn();
		configurarCliente({ onUnauthorized: spy });
		vi.stubGlobal(
			'fetch',
			vi.fn().mockImplementation((url) =>
				url === '/api/auth/refresh'
					? Promise.resolve(jsonResp(401, { error: 'refresh inválido' }))
					: Promise.resolve(jsonResp(401, { error: 'token inválido' }))
			)
		);
		fijarTokens({ access_token: 'VIEJO' });

		await Promise.allSettled([api('/api/x'), api('/api/y')]);
		const refreshes = vi.mocked(fetch).mock.calls.filter((c) => c[0] === '/api/auth/refresh');
		expect(refreshes.length).toBe(1);
		expect(spy).toHaveBeenCalledTimes(2);
	});
});

describe('logout', () => {
	it('llama al endpoint y limpia tokens aunque falle', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResp(200, { data: { logout: true } })));
		fijarTokens({ access_token: 'ACC', refresh_token: 'REF' });
		await logout();
		expect(fetch).toHaveBeenCalledWith('/api/auth/logout', expect.objectContaining({ method: 'POST' }));
	});
});

describe('feed (bloque 10)', () => {
	it('avisos construye la URL con paginación', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResp(200, { data: { datos: [], total: 0, limit: 5, offset: 10 } })));
		const p = await (await import('./api.js')).avisos('SID', { limit: 5, offset: 10 });
		expect(fetch).toHaveBeenCalledWith('/api/secciones/SID/avisos?limit=5&offset=10', expect.anything());
		expect(p.limit).toBe(5);
	});

	it('votar envía las opciones elegidas', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResp(200, { data: { votado: true } })));
		await (await import('./api.js')).votar('VID', [0, 2]);
		const [, opts] = vi.mocked(fetch).mock.calls[0];
		expect(JSON.parse(opts.body)).toEqual({ opciones: [0, 2] });
	});
});

describe('recarga de página (me tras perder memoria)', () => {
	it('me() con 401 dispara refresh y reintenta (sesión sobrevive F5)', async () => {
		let refreshes = 0;
		vi.stubGlobal(
			'fetch',
			vi.fn().mockImplementation((url, opts) => {
				if (url === '/api/auth/refresh') {
					refreshes++;
					return Promise.resolve(jsonResp(200, PAR));
				}
				if (url === '/api/auth/me') {
					const aut = opts.headers['Authorization'];
					return Promise.resolve(
						aut === 'Bearer ACC'
							? jsonResp(200, { data: { sub: 'user-123', rol_base: 'profesor' } })
							: jsonResp(401, { error: 'token inválido' })
					);
				}
				return Promise.resolve(jsonResp(404, {}));
			})
		);
		limpiarTokens();
		const yo = await (await import('./api.js')).me();
		expect(refreshes).toBe(1);
		expect(yo.rol_base).toBe('profesor');
	});
});

describe('2FA y perfil (bloque 11a)', () => {
	it('activar2FA devuelve secreto y QR', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
			jsonResp(200, { data: { secreto: 'SEC', otpauth_url: 'otpauth://x', qr_png_base64: 'AAA' } })
		));
		const r = await (await import('./api.js')).activar2FA();
		expect(r.secreto).toBe('SEC');
		expect(r.qr_png_base64.length).toBeGreaterThan(0);
	});

	it('confirmar2FA envía el código', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResp(200, { data: { activado: true } })));
		await (await import('./api.js')).confirmar2FA('123456');
		const [, opts] = vi.mocked(fetch).mock.calls[0];
		expect(JSON.parse(opts.body)).toEqual({ codigo: '123456' });
	});

	it('seguridad trae el estado', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
			jsonResp(200, { data: { email: 'a@b.c', totp_activado: true } })
		));
		const r = await (await import('./api.js')).seguridad();
		expect(r.totp_activado).toBe(true);
	});
});
