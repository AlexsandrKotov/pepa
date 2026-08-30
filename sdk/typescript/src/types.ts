/**
 * Core type definitions for PEPA plugins
 */

export interface PluginManifest {
  name: string;
  version: string;
  description?: string;
  author?: string;
  homepage?: string;
  icon?: string;
  
  // Plugin capabilities
  provides?: {
    pages?: PageDefinition[];
    widgets?: WidgetDefinition[];
    apiRoutes?: APIRouteDefinition[];
    eventHandlers?: EventHandlerDefinition[];
  };
  
  // Dependencies
  requires?: {
    plugins?: string[];
    permissions?: string[];
  };
  
  // Configuration
  config?: ConfigDefinition[];
}

export interface PageDefinition {
  path: string;
  title: string;
  component: React.ComponentType;
  icon?: string;
  requiresAuth?: boolean;
  permissions?: string[];
}

export interface WidgetDefinition {
  id: string;
  title: string;
  component: React.ComponentType;
  size?: 'small' | 'medium' | 'large';
  permissions?: string[];
}

export interface APIRouteDefinition {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  path: string;
  handler: APIHandler;
  permissions?: string[];
}

export interface EventHandlerDefinition {
  event: string;
  handler: EventHandler;
}

export interface ConfigDefinition {
  key: string;
  type: 'string' | 'number' | 'boolean' | 'json';
  label: string;
  description?: string;
  required?: boolean;
  default?: any;
}

export type APIHandler = (req: APIRequest) => Promise<APIResponse>;

export type EventHandler = (event: PluginEvent) => Promise<void>;

export interface APIRequest {
  method: string;
  path: string;
  headers: Record<string, string>;
  query: Record<string, string>;
  body?: any;
  user?: UserContext;
}

export interface APIResponse {
  status: number;
  headers?: Record<string, string>;
  body?: any;
}

export interface PluginEvent {
  type: string;
  payload: any;
  timestamp: number;
  tenantId: string;
  userId?: string;
}

export interface UserContext {
  id: string;
  email: string;
  name: string;
  roles: string[];
  permissions: string[];
  tenantId: string;
}

export interface PluginContext {
  manifest: PluginManifest;
  config: Record<string, any>;
  api: PluginAPI;
}

export interface PluginAPI {
  // HTTP client
  http: {
    get<T>(path: string, options?: RequestOptions): Promise<T>;
    post<T>(path: string, body?: any, options?: RequestOptions): Promise<T>;
    put<T>(path: string, body?: any, options?: RequestOptions): Promise<T>;
    delete<T>(path: string, options?: RequestOptions): Promise<T>;
  };
  
  // Storage
  storage: {
    get<T>(key: string): Promise<T | null>;
    set<T>(key: string, value: T): Promise<void>;
    delete(key: string): Promise<void>;
    list(prefix?: string): Promise<string[]>;
  };
  
  // Events
  events: {
    emit(event: string, payload: any): Promise<void>;
    on(event: string, handler: EventHandler): () => void;
  };
  
  // Logging
  logger: {
    debug(message: string, ...args: any[]): void;
    info(message: string, ...args: any[]): void;
    warn(message: string, ...args: any[]): void;
    error(message: string, ...args: any[]): void;
  };
  
  // User context
  getUser(): UserContext | null;
  getTenantId(): string;
}

export interface RequestOptions {
  headers?: Record<string, string>;
  query?: Record<string, string>;
  timeout?: number;
}
