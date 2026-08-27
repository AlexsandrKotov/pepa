# PEPA Vault Integration Guide

## Обзор

Интеграция PEPA с HashiCorp Vault для безопасного управления секретами, credentials и sensitive данными.

---

## Возможности

### 1. Vault Secret Browser 🗝️

Интерактивный браузер для навигации по Vault и выбора секретов.

**Функции**:
- ✅ Browse Vault paths (древовидная навигация)
- ✅ List secrets at path
- ✅ Preview secret keys (без показа значений)
- ✅ Select secrets for import
- ✅ Auto-generate Kubernetes Secrets
- ✅ Inject в deployments

**Использование**:
```typescript
// При создании сервиса
1. Нажать "Select from Vault"
2. Browse к нужному пути (например, secret/data/prod/database)
3. Выбрать нужные ключи (username, password)
4. Click "Import"
5. PEPA автоматически создаст K8s Secret и inject в deployment
```

---

### 2. Dynamic Credentials 🔄

Динамическая генерация credentials через Vault.

#### AWS Credentials
```yaml
# Как работает:
1. PEPA запрашивает у Vault временные AWS credentials
2. Vault создает access key и secret key
3. Credentials действительны 15 минут
4. Автоматическая ротация
5. Audit log всех запросов

# Использование:
- Path: aws/creds/readonly
- Result: access_key, secret_key
- TTL: 15 minutes
- Auto-rotation: enabled
```

#### Database Credentials
```yaml
# Как работает:
1. PEPA запрашивает у Vault DB credentials
2. Vault создает временного пользователя в PostgreSQL
3. Credentials действительны 1 час
4. Автоматическая ротация
5. Automatic cleanup после истечения

# Использование:
- Path: database/creds/readonly
- Result: username, password
- TTL: 1 hour
- Auto-rotation: enabled
- Database: PostgreSQL, MySQL, MongoDB
```

#### PKI Certificates
```yaml
# Как работает:
1. PEPA запрашивает у Vault TLS сертификат
2. Vault генерирует сертификат с помощью CA
3. Сертификат действителен 30 дней
4. Автоматическое продление
5. Integration с cert-manager

# Использование:
- Path: pki/issue/example-dot-com
- Result: certificate, private_key, ca_chain
- TTL: 30 days
- Auto-renewal: enabled
```

#### SSH Keys
```yaml
# Как работает:
1. PEPA запрашивает у Vault SSH key
2. Vault генерирует key pair
3. Public key подписывается Vault SSH CA
4. Key действителен 1 день
5. Audit trail

# Использование:
- Path: ssh/sign/otp-key-role
- Result: signed_key, private_key
- TTL: 1 day
- Signed by: Vault SSH CA
```

---

### 3. Secret Injection 📥

Автоматическая инъекция секретов в Kubernetes deployments.

#### Environment Variables
```yaml
# Deployment с Vault secrets
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: DB_USERNAME
          valueFrom:
            secretKeyRef:
              name: vault-secrets
              key: db-username
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: vault-secrets
              key: db-password
```

#### Mounted Files
```yaml
# Deployment с mounted secrets
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        volumeMounts:
        - name: vault-secrets
          mountPath: /etc/secrets
          readOnly: true
      volumes:
      - name: vault-secrets
        secret:
          secretName: vault-secrets
```

---

### 4. Security Features 🔒

#### Audit Logging
```yaml
# Все обращения к Vault логируются:
- Who: пользователь или сервис
- What: какой секрет запрошен
- When: timestamp
- Where: IP address
- Result: success/failure

# Пример audit log:
{
  "time": "2026-08-11T10:30:00Z",
  "user": "developer@example.com",
  "action": "read",
  "path": "secret/data/prod/database",
  "keys": ["username", "password"],
  "result": "success",
  "ip": "192.168.1.100"
}
```

#### RBAC (Role-Based Access Control)
```yaml
# Vault policies для PEPA:
# Policy: pepa-read-secrets
path "secret/data/prod/*" {
  capabilities = ["read", "list"]
}

path "secret/data/staging/*" {
  capabilities = ["read", "list"]
}

path "aws/creds/*" {
  capabilities = ["read"]
}

path "database/creds/*" {
  capabilities = ["read"]
}
```

#### Encryption
```yaml
# Encryption at rest:
- Все секреты шифруются в Vault
- AES-256-GCM encryption
- Key rotation support

# Encryption in transit:
- TLS 1.3 для всех connections
- Certificate validation
- Mutual TLS (опционально)
```

#### Secret Versioning
```yaml
# Vault KV v2 поддерживает versioning:
- Каждая запись имеет версию
- Можно откатиться к предыдущей версии
- Soft delete (можно восстановить)
- Metadata: кто, когда, что изменил

# Пример:
$ vault kv metadata get secret/data/prod/database
======= Metadata =======
Key              Value
---              -----
created_time     2026-08-11T10:00:00Z
current_version  3
updated_time     2026-08-11T12:00:00Z

======= Versions =======
Key  Value
---  -----
1    2026-08-11T10:00:00Z
2    2026-08-11T11:00:00Z
3    2026-08-11T12:00:00Z
```

---

## API Endpoints

### Vault Browser
```
GET  /api/v1/vault/browse              - Browse Vault paths
GET  /api/v1/vault/browse/:path        - List secrets at path
POST /api/v1/vault/browse/import       - Import secrets to environment
GET  /api/v1/vault/policies            - List Vault policies
POST /api/v1/vault/dynamic-credentials - Request dynamic credentials
```

### Vault Management
```
GET  /api/v1/vault/paths               - List all paths
GET  /api/v1/vault/secrets/:path       - Get secret by path
POST /api/v1/vault/secrets/:path       - Create/update secret
GET  /api/v1/vault/engines             - List secret engines
POST /api/v1/vault/test-connection     - Test Vault connection
```

---

## Frontend Components

### VaultSecretPicker
```typescript
// Компонент для выбора секретов
<VaultSecretPicker
  environment="production"
  onSelect={(secrets) => {
    console.log('Selected secrets:', secrets);
  }}
/>
```

**Функции**:
- Breadcrumb navigation
- Secret list с checkboxes
- Preview secret keys
- Import button
- Search/filter

### VaultStatusDashboard
```typescript
// Dashboard для мониторинга Vault
<VaultStatusDashboard
  showMetrics={true}
  showAuditLog={true}
  showDynamicCredentials={true}
/>
```

**Функции**:
- Vault health status
- Secret count by path
- Dynamic credentials usage
- Audit log viewer
- Policy violations

---

## Use Cases

### 1. Database Credentials
```yaml
# Сценарий:
1. Разработчик создает сервис
2. Нужен доступ к PostgreSQL
3. Открывает Vault Secret Picker
4. Browse к secret/data/prod/database
5. Выбирает username, password
6. Click "Import"
7. PEPA создает K8s Secret
8. Inject в deployment как env vars
9. Сервис может подключиться к БД

# Результат:
- Секреты не хранятся в Git
- Автоматическая ротация
- Audit log
- Безопасность
```

### 2. API Keys
```yaml
# Сценарий:
1. Сервису нужен Stripe API key
2. Открывает Vault Secret Picker
3. Browse к secret/data/api-keys/stripe
4. Выбирает api_key
5. Click "Import"
6. PEPA создает K8s Secret
7. Inject в deployment
8. Сервис может использовать Stripe

# Результат:
- API keys безопасны
- Централизованное управление
- Audit log
- Easy rotation
```

### 3. TLS Certificates
```yaml
# Сценарий:
1. Нужен TLS сертификат для Ingress
2. Открывает Vault Secret Picker
3. Browse к pki/issue/example-dot-com
4. Выбирает certificate, private_key
5. Click "Import"
6. PEPA создает K8s TLS Secret
7. Используется в Ingress
8. Автоматическое продление

# Результат:
- Certificates управляются централизованно
- Автоматическое продление
- Нет manual intervention
- Security best practices
```

### 4. Dynamic AWS Credentials
```yaml
# Сценарий:
1. Сервису нужен доступ к S3
2. PEPA запрашивает у Vault AWS credentials
3. Vault генерирует временные credentials
4. Credentials действительны 15 минут
5. Inject в pod
6. Сервис использует S3
7. Автоматическая ротация

# Результат:
- Временные credentials
- Автоматическая ротация
- Минимальный риск утечки
- Audit trail
```

---

## Configuration

### Vault Connection
```yaml
# config/vault.yaml
vault:
  address: "https://vault.example.com:8200"
  token: "s.xxxxxxxxxxxxx"  # или использовать Kubernetes auth
  namespace: "admin"
  
  # TLS settings
  tls:
    ca_cert: "/path/to/ca.crt"
    client_cert: "/path/to/client.crt"
    client_key: "/path/to/client.key"
    insecure: false
  
  # Timeout
  timeout: 30s
  
  # Retry
  retry:
    max_attempts: 3
    backoff: 1s
```

### Kubernetes Auth
```yaml
# Аутентификация через Kubernetes
vault:
  auth:
    method: "kubernetes"
    role: "pepa"
    service_account: "pepa-sa"
    
  # Vault будет проверять Kubernetes service account token
  # и выдавать Vault token для PEPA
```

---

## Security Best Practices

### 1. Least Privilege
```yaml
# Создавайте минимальные policies:
path "secret/data/prod/database" {
  capabilities = ["read"]  # только read, не write
}

# Не давайте root access:
path "secret/*" {
  capabilities = ["read", "list", "create", "update", "delete"]  # BAD!
}
```

### 2. Audit Everything
```yaml
# Включите audit devices:
$ vault audit enable file file_path=/var/log/vault/audit.log

# Все операции будут логироваться
```

### 3. Rotate Secrets
```yaml
# Настройте automatic rotation:
$ vault write database/roles/readonly \
    rotation_period=1h

# Секреты будут автоматически ротироваться
```

### 4. Use Dynamic Secrets
```yaml
# Вместо статических секретов используйте dynamic:
# BAD:
- Хранить DB password в Vault
- Вручную ротировать

# GOOD:
- Использовать database secrets engine
- Vault генерирует временные credentials
- Автоматическая ротация
```

### 5. Encrypt at Rest
```yaml
# Убедитесь что Vault шифрует данные:
$ vault status
# Sealed: false
# Encryption: AES-256-GCM
```

---

## Troubleshooting

### Problem: Cannot connect to Vault
```bash
# Проверьте:
1. Vault address правильный
2. Vault запущен
3. Network connectivity
4. TLS сертификаты валидны
5. Firewall rules

# Тест:
$ vault status -address=https://vault.example.com:8200
```

### Problem: Permission denied
```bash
# Проверьте:
1. Vault token валиден
2. Policy разрешает доступ
3. Path правильный
4. Namespace правильный

# Тест:
$ vault token lookup
$ vault policy read pepa-policy
```

### Problem: Secret not found
```bash
# Проверьте:
1. Path правильный
2. Secret существует
3. KV version правильный (v1 vs v2)
4. Namespace правильный

# Тест:
$ vault kv get secret/data/prod/database
```

---

## Integration Checklist

### Phase 1: Basic Integration
- [ ] Vault connection setup
- [ ] Authentication (token or K8s)
- [ ] Basic secret read
- [ ] Vault Secret Picker UI
- [ ] K8s Secret generation

### Phase 2: Advanced Features
- [ ] Dynamic credentials (AWS, DB)
- [ ] PKI certificates
- [ ] SSH keys
- [ ] Secret versioning
- [ ] Audit logging

### Phase 3: Security Hardening
- [ ] RBAC policies
- [ ] Encryption at rest
- [ ] TLS configuration
- [ ] Audit device setup
- [ ] Secret rotation

---

## Resources

- [Vault Documentation](https://www.vaultproject.io/docs)
- [Vault API Reference](https://www.vaultproject.io/api-docs)
- [Vault Kubernetes Integration](https://www.vaultproject.io/docs/platform/k8s)
- [Vault Agent Injector](https://www.vaultproject.io/docs/platform/k8s/injector)

---

**Создано**: 2026-08-11
**Версия**: 1.0
**Статус**: ✅ Готово к реализации
