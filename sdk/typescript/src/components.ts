/**
 * React components for PEPA plugins
 */

export interface PluginPageProps {
  children?: React.ReactNode;
}

export interface PluginWidgetProps {
  title?: string;
  children?: React.ReactNode;
}

/**
 * Plugin page wrapper component
 */
export function PluginPage({ children }: PluginPageProps) {
  return (
    <div className="plugin-page">
      {children}
    </div>
  );
}

/**
 * Plugin widget wrapper component
 */
export function PluginWidget({ title, children }: PluginWidgetProps) {
  return (
    <div className="plugin-widget">
      {title && <h3>{title}</h3>}
      {children}
    </div>
  );
}
