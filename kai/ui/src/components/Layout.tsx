import { useUIStore } from '../stores/uiStore'
import { Sidebar } from './Sidebar'

interface LayoutProps {
  children: React.ReactNode
}

/**
 * App shell: collapsible sidebar + scrollable main content area.
 */
export function Layout({ children }: LayoutProps) {
  const sidebarOpen = useUIStore((s) => s.sidebarOpen)
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)

  return (
    <div className="flex h-screen bg-gray-950 text-gray-100 overflow-hidden">
      {sidebarOpen && <Sidebar />}

      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {/* Topbar — shown when sidebar is closed */}
        {!sidebarOpen && (
          <div className="flex items-center px-4 py-3 border-b border-gray-800 bg-gray-900">
            <button
              onClick={toggleSidebar}
              className="text-gray-400 hover:text-gray-200 mr-3 transition-colors"
              aria-label="Open sidebar"
            >
              ☰
            </button>
            <span className="font-semibold text-sm text-gray-200">⚡ Kai</span>
          </div>
        )}

        <main className="flex-1 overflow-hidden">{children}</main>
      </div>
    </div>
  )
}
