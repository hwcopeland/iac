import { create } from 'zustand'

interface UIState {
  sidebarOpen: boolean
  sessionExpired: boolean
  setSidebarOpen: (open: boolean) => void
  setSessionExpired: (expired: boolean) => void
  toggleSidebar: () => void
}

export const useUIStore = create<UIState>((set) => ({
  sidebarOpen: true,
  sessionExpired: false,
  setSidebarOpen: (open) => set({ sidebarOpen: open }),
  setSessionExpired: (expired) => set({ sessionExpired: expired }),
  toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),
}))
