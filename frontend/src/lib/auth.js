// Store de sesión: { estado: 'anonimo'|'cargando'|'activa', usuario, forzarCambio }.
// Al cargar la app intenta recuperar la sesión vía /me (el refresh ocurre
// automáticamente en api.js si el access token está vencido y la cookie vive).
import { writable } from 'svelte/store';
import { configurarCliente, me, logout as apiLogout, limpiarTokens } from './api.js';

export const sesion = writable({ estado: 'cargando', usuario: null, forzarCambio: false });

configurarCliente({
	onUnauthorized: () => sesion.set({ estado: 'anonimo', usuario: null, forzarCambio: false })
});

export async function iniciarSesion() {
	try {
		const usuario = await me();
		sesion.set({ estado: 'activa', usuario, forzarCambio: false });
	} catch {
		sesion.set({ estado: 'anonimo', usuario: null, forzarCambio: false });
	}
}

export async function salir() {
	await apiLogout().catch(() => {});
	sesion.set({ estado: 'anonimo', usuario: null, forzarCambio: false });
}

// para pruebas y limpieza controlada
export function reiniciarSesion() {
	limpiarTokens();
	sesion.set({ estado: 'anonimo', usuario: null, forzarCambio: false });
}
