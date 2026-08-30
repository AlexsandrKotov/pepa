import { PluginManifest, PluginContext, PluginAPI } from './types';

/**
 * Define a PEPA plugin
 */
export function definePlugin(
  manifest: PluginManifest,
  setup: (context: PluginContext) => void | Promise<void>
): PluginDefinition {
  return {
    manifest,
    setup,
  };
}

export interface PluginDefinition {
  manifest: PluginManifest;
  setup: (context: PluginContext) => void | Promise<void>;
}

/**
 * Create plugin API client
 */
export function createPluginAPI(pluginName: string): PluginAPI {
  const baseURL = `/api/v1/plugins/${pluginName}`;
  
  return {
    http: {
      async get<T>(path: string, options?: any): Promise<T> {
        const response = await fetch(`${baseURL}${path}`, {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            ...options?.headers,
          },
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      },
      
      async post<T>(path: string, body?: any, options?: any): Promise<T> {
        const response = await fetch(`${baseURL}${path}`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...options?.headers,
          },
          body: body ? JSON.stringify(body) : undefined,
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      },
      
      async put<T>(path: string, body?: any, options?: any): Promise<T> {
        const response = await fetch(`${baseURL}${path}`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            ...options?.headers,
          },
          body: body ? JSON.stringify(body) : undefined,
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      },
      
      async delete<T>(path: string, options?: any): Promise<T> {
        const response = await fetch(`${baseURL}${path}`, {
          method: 'DELETE',
          headers: {
            'Content-Type': 'application/json',
            ...options?.headers,
          },
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      },
    },
    
    storage: {
      async get<T>(key: string): Promise<T | null> {
        const value = localStorage.getItem(`pepa:plugin:${pluginName}:${key}`);
        return value ? JSON.parse(value) : null;
      },
      
      async set<T>(key: string, value: T): Promise<void> {
        localStorage.setItem(`pepa:plugin:${pluginName}:${key}`, JSON.stringify(value));
      },
      
      async delete(key: string): Promise<void> {
        localStorage.removeItem(`pepa:plugin:${pluginName}:${key}`);
      },
      
      async list(prefix?: string): Promise<string[]> {
        const keys: string[] = [];
        for (let i = 0; i < localStorage.length; i++) {
          const key = localStorage.key(i);
          if (key?.startsWith(`pepa:plugin:${pluginName}:`)) {
            const pluginKey = key.replace(`pepa:plugin:${pluginName}:`, '');
            if (!prefix || pluginKey.startsWith(prefix)) {
              keys.push(pluginKey);
            }
          }
        }
        return keys;
      },
    },
    
    events: {
      async emit(event: string, payload: any): Promise<void> {
        window.dispatchEvent(new CustomEvent(`pepa:plugin:${pluginName}:${event}`, {
          detail: payload,
        }));
      },
      
      on(event: string, handler: (event: any) => Promise<void>): () => void {
        const listener = (e: CustomEvent) => {
          handler({
            type: event,
            payload: e.detail,
            timestamp: Date.now(),
            tenantId: '', // Will be filled by PEPA
          });
        };
        
        window.addEventListener(`pepa:plugin:${pluginName}:${event}`, listener as EventListener);
        
        return () => {
          window.removeEventListener(`pepa:plugin:${pluginName}:${event}`, listener as EventListener);
        };
      },
    },
    
    logger: {
      debug(message: string, ...args: any[]): void {
        console.debug(`[${pluginName}]`, message, ...args);
      },
      info(message: string, ...args: any[]): void {
        console.info(`[${pluginName}]`, message, ...args);
      },
      warn(message: string, ...args: any[]): void {
        console.warn(`[${pluginName}]`, message, ...args);
      },
      error(message: string, ...args: any[]): void {
        console.error(`[${pluginName}]`, message, ...args);
      },
    },
    
    getUser(): any {
      const userStr = localStorage.getItem('pepa:user');
      return userStr ? JSON.parse(userStr) : null;
    },
    
    getTenantId(): string {
      const user = this.getUser();
      return user?.tenantId || '';
    },
  };
}
