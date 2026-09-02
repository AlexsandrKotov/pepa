'use client';

import { useState } from 'react';

type Lang = 'en' | 'ru';

type DocBlockType = 'text' | 'code' | 'heading' | 'list' | 'table' | 'image';

interface DocBlock {
  type: DocBlockType;
  content: { en: string; ru: string };
  items?: { en: string; ru: string }[];
  headers?: { en: string; ru: string }[];
  rows?: { en: string[]; ru: string[] }[];
  src?: string;
  alt?: { en: string; ru: string };
}

interface DocSection {
  id: string;
  title: { en: string; ru: string };
  blocks: DocBlock[];
}

const sections: DocSection[] = [
  {
    id: 'getting-started',
    title: { en: 'Getting Started', ru: 'Начало работы' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA is a Platform Engineering & Pipeline Automator that provides a unified platform for service catalog management, GitOps deployment workflows, Kubernetes cluster operations, workflow automation, and developer self-service. It is built with Go, Next.js, PostgreSQL, and Kubernetes — designed for Platform Engineering teams who value developer experience.',
        ru: 'PEPA — это Platform Engineering & Pipeline Automator, предоставляющий единую платформу для управления каталогом сервисов, GitOps развёртываний, управления кластерами Kubernetes, автоматизации рабочих процессов и самообслуживания разработчиков. Платформа построена на Go, Next.js, PostgreSQL и Kubernetes — создана для команд платформенной инженерии, ценящих опыт разработчиков.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-dashboard.png', alt: { en: 'PEPA Dashboard', ru: 'Панель управления PEPA' }},
      { type: 'heading', content: { en: 'System Requirements', ru: 'Системные требования' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Component', ru: 'Компонент' }, { en: 'Minimum', ru: 'Минимум' }, { en: 'Recommended', ru: 'Рекомендуется' }],
        rows: [
          { en: ['CPU', '2 cores', '4+ cores'], ru: ['CPU', '2 ядра', '4+ ядер'] },
          { en: ['RAM', '4 GB', '8+ GB'], ru: ['Память', '4 ГБ', '8+ ГБ'] },
          { en: ['Disk', '20 GB', '50+ GB SSD'], ru: ['Диск', '20 ГБ', '50+ ГБ SSD'] },
          { en: ['Docker', '20.10+', 'Latest'], ru: ['Docker', '20.10+', 'Последняя'] },
          { en: ['Docker Compose', 'v2+', 'Latest'], ru: ['Docker Compose', 'v2+', 'Последняя'] },
        ],
      },
      { type: 'heading', content: { en: 'Quick Start (Docker)', ru: 'Быстрый старт (Docker)' }},
      { type: 'code', content: {
        en: '# 1. Clone the repository\ngit clone https://github.com/AlexsandrKotov/pepa.git\ncd pepa\n\n# 2. Copy environment file and adjust if needed\ncp .env.example .env\n\n# 3. Build and start all services\nmake docker-up\n\n# 4. Wait ~30 seconds for all services to start\n# Check status:\ndocker compose -f deployments/compose/docker-compose.yml ps\n\n# 5. Access the portal\n# Frontend: http://localhost:3000\n# API:      http://localhost:8088\n# MinIO:    http://localhost:9001 (minioadmin/minioadmin)',
        ru: '# 1. Клонируйте репозиторий\ngit clone https://github.com/AlexsandrKotov/pepa.git\ncd pepa\n\n# 2. Скопируйте файл окружения и при необходимости измените\ncp .env.example .env\n\n# 3. Соберите и запустите все сервисы\nmake docker-up\n\n# 4. Подождите ~30 секунд пока все сервисы запустятся\n# Проверка статуса:\ndocker compose -f deployments/compose/docker-compose.yml ps\n\n# 5. Откройте портал\n# Frontend: http://localhost:3000\n# API:      http://localhost:8088\n# MinIO:    http://localhost:9001 (minioadmin/minioadmin)',
      }},
      { type: 'heading', content: { en: 'Verify Installation', ru: 'Проверка установки' }},
      { type: 'code', content: {
        en: '# Check API health\ncurl http://localhost:8088/healthz\n# Expected: {"status":"ok","version":"0.1.0"}\n\n# Check all containers are running\ndocker compose -f deployments/compose/docker-compose.yml ps\n# All should show "Up" status\n\n# Check frontend is accessible\ncurl -s -o /dev/null -w "%{http_code}" http://localhost:3000\n# Expected: 200',
        ru: '# Проверка здоровья API\ncurl http://localhost:8088/healthz\n# Ожидается: {"status":"ok","version":"0.1.0"}\n\n# Проверьте что все контейнеры запущены\ndocker compose -f deployments/compose/docker-compose.yml ps\n# Все должны показывать статус "Up"\n\n# Проверьте доступность фронтенда\ncurl -s -o /dev/null -w "%{http_code}" http://localhost:3000\n# Ожидается: 200',
      }},
      { type: 'heading', content: { en: 'First Login', ru: 'Первый вход' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Open http://localhost:3000 in your browser', ru: 'Откройте http://localhost:3000 в браузере' },
        { en: 'You will be redirected to the login page', ru: 'Вы будете перенаправлены на страницу входа' },
        { en: 'In development mode (DEV_MODE=true), authentication is disabled — you are logged in automatically as Admin', ru: 'В режиме разработки (DEV_MODE=true) аутентификация отключена — вы автоматически входите как Admin' },
        { en: 'Explore the Dashboard, Services, Clusters, and Settings pages', ru: 'Изучите страницы Dashboard, Services, Clusters и Settings' },
        { en: 'Click the Admin avatar (top-right) to access Settings, Roles, and Documentation', ru: 'Нажмите на аватар Admin (вверху справа) для доступа к Settings, Roles и Documentation' },
      ]},
      { type: 'heading', content: { en: 'Local Development', ru: 'Локальная разработка' }},
      { type: 'code', content: {
        en: '# Start infrastructure only (PostgreSQL, Redis, MinIO)\ndocker compose -f deployments/compose/docker-compose.yml up -d postgres redis minio minio-init\n\n# Build all Go binaries\nmake build\n\n# Start API server (terminal 1)\nmake run-dev\n\n# Start background worker (terminal 2)\nmake run-worker\n\n# Start frontend dev server (terminal 3)\ncd frontend && npm install && npm run dev\n\n# Frontend: http://localhost:3000 (hot reload)\n# API:      http://localhost:8080',
        ru: '# Запуск только инфраструктуры (PostgreSQL, Redis, MinIO)\ndocker compose -f deployments/compose/docker-compose.yml up -d postgres redis minio minio-init\n\n# Сборка всех Go бинарников\nmake build\n\n# Запуск API сервера (терминал 1)\nmake run-dev\n\n# Запуск фонового воркера (терминал 2)\nmake run-worker\n\n# Запуск dev сервера фронтенда (терминал 3)\ncd frontend && npm install && npm run dev\n\n# Frontend: http://localhost:3000 (горячая перезагрузка)\n# API:      http://localhost:8080',
      }},
      { type: 'heading', content: { en: 'Architecture', ru: 'Архитектура' }},
      { type: 'code', content: {
        en: '┌─────────────┐     ┌──────────────┐     ┌──────────────┐\n│   Frontend   │────▶│  API Server  │────▶│  PostgreSQL  │\n│  (Next.js)   │     │   (Gin/Go)   │     │  + PGvector  │\n│  Port 3000   │     │  Port 8080   │     │  Port 5432   │\n└─────────────┘     └──────┬───────┘     └──────────────┘\n                           │\n                    ┌──────┴───────┐\n              ┌─────▼─────┐ ┌─────▼─────┐\n              │   Redis    │ │   MinIO   │\n              │ (Queue)    │ │ (S3/Art.) │\n              └───────────┘ └───────────┘\n\nProduction (with Nginx reverse proxy + TLS):\n\n  Client → Nginx (:80/:443) → Frontend (:3000)\n                           → API (:8080)\n                           → Worker (background)',
        ru: '┌─────────────┐     ┌──────────────┐     ┌──────────────┐\n│   Фронтенд   │────▶│  API Сервер  │────▶│  PostgreSQL  │\n│  (Next.js)   │     │   (Gin/Go)   │     │  + PGvector  │\n│  Порт 3000   │     │  Порт 8080   │     │  Порт 5432   │\n└─────────────┘     └──────┬───────┘     └──────────────┘\n                           │\n                    ┌──────┴───────┐\n              ┌─────▼─────┐ ┌─────▼─────┐\n              │   Redis    │ │   MinIO   │\n              │ (Очередь)  │ │ (S3/Арт.) │\n              └───────────┘ └───────────┘\n\nПродакшен (с Nginx reverse proxy + TLS):\n\n  Клиент → Nginx (:80/:443) → Frontend (:3000)\n                           → API (:8080)\n                           → Worker (фоновый)',
      }},
    ],
  },
  {
    id: 'auth',
    title: { en: 'Authentication & Admin', ru: 'Авторизация и администрирование' },
    blocks: [
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-login.png', alt: { en: 'PEPA Login page', ru: 'Страница входа PEPA' }},
      { type: 'heading', content: { en: 'Default Admin Account', ru: 'Учётная запись администратора' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Field', ru: 'Поле' }, { en: 'Value', ru: 'Значение' }],
        rows: [
          { en: ['Email', 'admin@pepa.dev'], ru: ['Email', 'admin@pepa.dev'] },
          { en: ['Role', 'Platform Admin'], ru: ['Роль', 'Platform Admin'] },
        ],
      },
      { type: 'heading', content: { en: 'Logging In', ru: 'Вход в систему' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Navigate to http://localhost:3000/login', ru: 'Откройте http://localhost:3000/login' },
        { en: 'Enter admin email and password', ru: 'Введите email администратора и пароль' },
        { en: 'You will be redirected to the Dashboard', ru: 'Вы попадёте на панель управления' },
      ]},
      { type: 'heading', content: { en: 'Authentication Modes', ru: 'Режимы аутентификации' }},
      { type: 'text', content: {
        en: 'PEPA supports two authentication modes: Development Mode (DEV_MODE=true) bypasses authentication for local testing, and Production Mode (DEV_MODE=false) requires JWT-based authentication with secure password hashing.',
        ru: 'PEPA поддерживает два режима аутентификации: Режим разработки (DEV_MODE=true) пропускает аутентификацию для локального тестирования, и Продакшен режим (DEV_MODE=false) требует JWT аутентификации с безопасным хешированием паролей.',
      }},
      { type: 'heading', content: { en: 'JWT Token Management', ru: 'Управление JWT токенами' }},
      { type: 'text', content: {
        en: 'In production mode, PEPA uses JWT tokens for session management. Tokens are stored in HTTP-only cookies for security. Access tokens expire after 24 hours, refresh tokens after 7 days.',
        ru: 'В продакшен режиме PEPA использует JWT токены для управления сессиями. Токены хранятся в HTTP-only cookies для безопасности. Токены доступа истекают через 24 часа, токены обновления через 7 дней.',
      }},
      { type: 'heading', content: { en: 'User Management (Settings → Users)', ru: 'Управление пользователями (Настройки → Пользователи)' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Create new users with name, email, password, and role', ru: 'Создание новых пользователей с именем, email, паролем и ролью' },
        { en: 'Activate / deactivate user accounts', ru: 'Активация / деактивация учётных записей' },
        { en: 'Reset user passwords', ru: 'Сброс паролей пользователей' },
        { en: 'Search and filter users', ru: 'Поиск и фильтрация пользователей' },
        { en: 'Assign users to teams for resource-level access control', ru: 'Назначение пользователей в команды для контроля доступа на уровне ресурсов' },
      ]},
      { type: 'heading', content: { en: 'Password Policies', ru: 'Политики паролей' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Minimum 8 characters required', ru: 'Минимум 8 символов' },
        { en: 'At least one uppercase letter, one lowercase letter, one number', ru: 'Хотя бы одна заглавная буква, одна строчная, одна цифра' },
        { en: 'Password history prevents reuse of last 5 passwords', ru: 'История паролей предотвращает повторное использование последних 5 паролей' },
        { en: 'Failed login attempts are logged for security audit', ru: 'Неудачные попытки входа логируются для аудита безопасности' },
      ]},
      { type: 'heading', content: { en: 'Default Roles', ru: 'Роли по умолчанию' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Role', ru: 'Роль' }, { en: 'Access', ru: 'Доступ' }],
        rows: [
          { en: ['Admin', 'Full access to all resources'], ru: ['Admin', 'Полный доступ ко всем ресурсам'] },
          { en: ['Developer', 'Create/read/update services, workflows, deployments'], ru: ['Developer', 'Создание/чтение/обновление сервисов, рабочих процессов, развёртываний'] },
          { en: ['Viewer', 'Read-only access to all resources'], ru: ['Viewer', 'Только чтение всех ресурсов'] },
        ],
      },
    ],
  },
  {
    id: 'dashboard',
    title: { en: 'Dashboard', ru: 'Панель управления' },
    blocks: [
      { type: 'text', content: {
        en: 'The Dashboard is the main landing page showing a platform overview with key metrics and recent activity. It provides a quick glance at the health of your entire platform.',
        ru: 'Панель управления — главная страница с обзором платформы, ключевыми метриками и последней активностью. Она даёт быстрый обзор состояния всей платформы.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-dashboard.png', alt: { en: 'Dashboard with key metrics and recent activity', ru: 'Панель управления с ключевыми метриками и последней активностью' }},
      { type: 'heading', content: { en: 'Key Metrics', ru: 'Ключевые метрики' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Metric', ru: 'Метрика' }, { en: 'Description', ru: 'Описание' }, { en: 'Click Action', ru: 'Действие при клике' }],
        rows: [
          { en: ['Services', 'Total registered services count', 'Navigate to Services page'], ru: ['Сервисы', 'Общее количество зарегистрированных сервисов', 'Перейти на страницу Сервисы'] },
          { en: ['Clusters', 'Connected Kubernetes clusters', 'Navigate to Clusters page'], ru: ['Кластеры', 'Подключённые кластеры Kubernetes', 'Перейти на страницу Кластеры'] },
          { en: ['Deployments', 'Active deployments across environments', 'Navigate to Deployments page'], ru: ['Развёртывания', 'Активные развёртывания во всех окружениях', 'Перейти на страницу Развёртывания'] },
          { en: ['Pipelines', 'Configured CI/CD pipelines', 'Navigate to Pipelines page'], ru: ['Пайплайны', 'Настроенные CI/CD пайплайны', 'Перейти на страницу Пайплайны'] },
          { en: ['AI Chat', 'AI Assistant quick access', 'Open AI chat panel'], ru: ['AI Чат', 'Быстрый доступ к AI Ассистенту', 'Открыть панель AI чата'] },
        ],
      },
      { type: 'heading', content: { en: 'Dashboard Layout', ru: 'Структура панели' }},
      { type: 'code', content: {
        en: '┌─────────────────────────────────────────────────────┐\n│  Dashboard                                          │\n├──────────┬──────────┬──────────┬──────────┬─────────┤\n│ Services │ Clusters │Deployments│Pipelines │  AI    │\n│    12    │     3    │    47    │     8    │ Chat   │\n├──────────┴──────────┴──────────┴──────────┴─────────┤\n│  Deployment Frequency Chart                         │\n│  ████████░░ 78%                                     │\n│  Recent Activity                                    │\n│  • api-gateway deployed to production    2m ago     │\n│  • user-service scaled to 3 replicas     15m ago    │\n└─────────────────────────────────────────────────────┘',
        ru: '┌─────────────────────────────────────────────────────┐\n│  Панель управления                                  │\n├──────────┬──────────┬──────────┬──────────┬─────────┤\n│ Сервисы  │Кластеры  │Развёртыв.│Пайплайны │  AI    │\n│    12    │     3    │    47    │     8    │ Чат    │\n├──────────┴──────────┴──────────┴──────────┴─────────┤\n│  График частоты развёртываний                       │\n│  ████████░░ 78%                                     │\n│  Последняя активность                               │\n│  • api-gateway развернут в production    2 мин назад│\n│  • user-service масштабирован до 3       15 мин назад│\n└─────────────────────────────────────────────────────┘',
      }},
      { type: 'heading', content: { en: 'Quick Actions', ru: 'Быстрые действия' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Click "Deploy Service" to create a new service from a template', ru: 'Нажмите "Развернуть сервис" для создания нового сервиса из шаблона' },
        { en: 'Use the search bar (Cmd+K / Ctrl+K) to quickly find services, clusters, or settings', ru: 'Используйте строку поиска (Cmd+K / Ctrl+K) для быстрого поиска сервисов, кластеров или настроек' },
        { en: 'Click notification bell to see recent deployments, policy violations, and system alerts', ru: 'Нажмите на колокольчик уведомлений для просмотра последних развёртываний, нарушений политик и системных оповещений' },
        { en: 'Use the theme toggle (sun/moon icon) to switch between light and dark mode', ru: 'Используйте переключатель темы (значок солнца/луны) для переключения между светлой и тёмной темой' },
      ]},
      { type: 'heading', content: { en: 'Recent Activity Feed', ru: 'Лента последней активности' }},
      { type: 'text', content: {
        en: 'The activity feed shows the most recent platform events: deployments, pipeline runs, workflow executions, scorecard evaluations, and user actions. Each entry is clickable to navigate to the relevant detail page.',
        ru: 'Лента активности показывает последние события платформы: развёртывания, запуски пайплайнов, выполнение рабочих процессов, оценки карт и действия пользователей. Каждая запись кликабельна для перехода на страницу деталей.',
      }},
    ],
  },
  {
    id: 'services',
    title: { en: 'Service Catalog', ru: 'Каталог сервисов' },
    blocks: [
      { type: 'text', content: {
        en: 'The Service Catalog is the heart of PEPA. It contains all registered services with their metadata, ownership, health status, deployment history, and scorecard evaluation. Navigate to Services to see the full list with filtering and search capabilities.',
        ru: 'Каталог сервисов — это ядро PEPA. Он содержит все зарегистрированные сервисы с их метаданными, владельцами, статусом здоровья, историей развёртываний и оценкой по карте. Перейдите в Сервисы для просмотра полного списка с фильтрацией и поиском.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-services-list.png', alt: { en: 'Service Catalog with search and filters', ru: 'Каталог сервисов с поиском и фильтрами' }},
      { type: 'heading', content: { en: 'Creating a Service', ru: 'Создание сервиса' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Click Deploy or Services → Deploy Service', ru: 'Нажмите Развернуть или Сервисы → Развернуть сервис' },
        { en: 'Choose a template: Node.js API, Go API, Python API, Static Site, or Helm Import', ru: 'Выберите шаблон: Node.js API, Go API, Python API, Static Site или Helm Import' },
        { en: 'Fill in service name, team, description, and configuration', ru: 'Заполните имя сервиса, команду, описание и конфигурацию' },
        { en: 'Select target environment (staging, production) and cluster', ru: 'Выберите целевое окружение (staging, production) и кластер' },
        { en: 'Click Deploy — the service will be created and optionally deployed to the cluster', ru: 'Нажмите Развернуть — сервис будет создан и при необходимости развернут в кластере' },
      ]},
      { type: 'heading', content: { en: 'Service Templates', ru: 'Шаблоны сервисов' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Template', ru: 'Шаблон' }, { en: 'Language', ru: 'Язык' }, { en: 'Resources', ru: 'Ресурсы' }, { en: 'Includes', ru: 'Включает' }],
        rows: [
          { en: ['Node.js API', 'JavaScript', '200m CPU, 256Mi RAM', 'Express app + Dockerfile + Helm chart'], ru: ['Node.js API', 'JavaScript', '200m CPU, 256Mi RAM', 'Express приложение + Dockerfile + Helm чарт'] },
          { en: ['Go API', 'Go', '100m CPU, 128Mi RAM', 'Gin app + Dockerfile + Helm chart'], ru: ['Go API', 'Go', '100m CPU, 128Mi RAM', 'Gin приложение + Dockerfile + Helm чарт'] },
          { en: ['Python API', 'Python', '200m CPU, 256Mi RAM', 'FastAPI app + Dockerfile + Helm chart'], ru: ['Python API', 'Python', '200m CPU, 256Mi RAM', 'FastAPI приложение + Dockerfile + Helm чарт'] },
          { en: ['Static Site', 'HTML', '50m CPU, 64Mi RAM', 'Nginx + static files + Helm chart'], ru: ['Static Site', 'HTML', '50m CPU, 64Mi RAM', 'Nginx + статические файлы + Helm чарт'] },
          { en: ['Helm Import', 'Any', '100m CPU, 128Mi RAM', 'Import existing Helm chart from repo'], ru: ['Helm Import', 'Любой', '100m CPU, 128Mi RAM', 'Импорт существующего Helm чарта из репо'] },
        ],
      },
      { type: 'heading', content: { en: 'Service Details', ru: 'Детали сервиса' }},
      { type: 'text', content: {
        en: 'Click any service to see: metadata (owner, team, lifecycle stage), relationships (graph view), deployment history, health checks, and scorecard evaluation. The service detail page provides a comprehensive view of the service\'s current state.',
        ru: 'Нажмите на любой сервис для просмотра: метаданные (владелец, команда, стадия жизненного цикла), связи (граф), история развёртываний, проверки здоровья и оценка по карте. Страница деталей сервиса предоставляет полный обзор его текущего состояния.',
      }},
      { type: 'heading', content: { en: 'Service Lifecycle Stages', ru: 'Стадии жизненного цикла сервиса' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Stage', ru: 'Стадия' }, { en: 'Description', ru: 'Описание' }],
        rows: [
          { en: ['Development', 'Service is under active development'], ru: ['Разработка', 'Сервис в активной разработке'] },
          { en: ['Staging', 'Deployed to staging for testing'], ru: ['Staging', 'Развернут в staging для тестирования'] },
          { en: ['Production', 'Live in production environment'], ru: ['Production', 'Работает в продакшен окружении'] },
          { en: ['Deprecated', 'Scheduled for decommission'], ru: ['Устаревший', 'Запланирован на вывод из эксплуатации'] },
          { en: ['Decommissioned', 'No longer active'], ru: ['Выведен', 'Больше не активен'] },
        ],
      },
      { type: 'heading', content: { en: 'Service Health Checks', ru: 'Проверки здоровья сервиса' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Health endpoint (GET /healthz) — returns 200 if service is running', ru: 'Health endpoint (GET /healthz) — возвращает 200 если сервис работает' },
        { en: 'Readiness endpoint (GET /readyz) — returns 200 if service can accept traffic', ru: 'Readiness endpoint (GET /readyz) — возвращает 200 если сервис готов принимать трафик' },
        { en: 'PEPA polls these endpoints every 30 seconds and displays status in the catalog', ru: 'PEPA опрашивает эти endpoints каждые 30 секунд и отображает статус в каталоге' },
        { en: 'Failed health checks trigger alerts and affect scorecard evaluation', ru: 'Проваленные проверки здоровья вызывают оповещения и влияют на оценку по карте' },
      ]},
    ],
  },
  {
    id: 'clusters',
    title: { en: 'Kubernetes Clusters', ru: 'Кластеры Kubernetes' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA connects to your Kubernetes clusters to provide unified management, deployment, and monitoring. Multiple clusters are supported across different environments (staging, production, etc.).',
        ru: 'PEPA подключается к вашим кластерам Kubernetes для обеспечения единого управления, развёртывания и мониторинга. Поддерживаются несколько кластеров в разных окружениях (staging, production и т.д.).',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-clusters.png', alt: { en: 'Kubernetes Clusters overview', ru: 'Обзор кластеров Kubernetes' }},
      { type: 'heading', content: { en: 'Adding a Cluster', ru: 'Добавление кластера' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to Clusters → Add Cluster', ru: 'Перейдите в Кластеры → Добавить кластер' },
        { en: 'Upload kubeconfig file or paste contents directly', ru: 'Загрузите файл kubeconfig или вставьте его содержимое напрямую' },
        { en: 'Platform discovers cluster name, endpoint, namespaces, and nodes automatically', ru: 'Платформа автоматически обнаружит имя кластера, endpoint, namespaces и узлы' },
        { en: 'Click Connect — PEPA will verify connectivity and display cluster status', ru: 'Нажмите Подключить — PEPA проверит связность и отобразит статус кластера' },
        { en: 'Optionally assign the cluster to an environment (staging, production)', ru: 'При необходимости назначьте кластер окружению (staging, production)' },
      ]},
      { type: 'heading', content: { en: 'Cluster View', ru: 'Просмотр кластера' }},
      { type: 'code', content: {
        en: '┌─────────────────────────────────────────────────┐\n│  Cluster: production-east                       │\n├─────────────────────────────────────────────────┤\n│  Status:  ● Connected                           │\n│  Endpoint: https://k8s.example.com:6443         │\n│  Version:  v1.28.3                              │\n├─────────────────────────────────────────────────┤\n│  Nodes (3)          Namespaces (5)              │\n│  ├─ node-1 (4 CPU)  ├─ default                 │\n│  ├─ node-2 (4 CPU)  ├─ production              │\n│  └─ node-3 (8 CPU)  ├─ staging                 │\n├─────────────────────────────────────────────────┤\n│  CPU:    ████████░░ 65%                         │\n│  Memory: ██████░░░░ 48%                         │\n└─────────────────────────────────────────────────┘',
        ru: '┌─────────────────────────────────────────────────┐\n│  Кластер: production-east                       │\n├─────────────────────────────────────────────────┤\n│  Статус:   ● Подключён                          │\n│  Endpoint: https://k8s.example.com:6443         │\n│  Версия:   v1.28.3                              │\n├─────────────────────────────────────────────────┤\n│  Узлы (3)           Namespaces (5)              │\n│  ├─ node-1 (4 CPU)  ├─ default                 │\n│  ├─ node-2 (4 CPU)  ├─ production              │\n│  └─ node-3 (8 CPU)  ├─ staging                 │\n├─────────────────────────────────────────────────┤\n│  CPU:    ████████░░ 65%                         │\n│  Память: ██████░░░░ 48%                         │\n└─────────────────────────────────────────────────┘',
      }},
      { type: 'heading', content: { en: 'Health Monitoring', ru: 'Мониторинг здоровья' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Status', ru: 'Статус' }, { en: 'Indicator', ru: 'Индикатор' }, { en: 'Meaning', ru: 'Значение' }],
        rows: [
          { en: ['Connected', 'Green', 'API server reachable, all checks pass'], ru: ['Подключён', 'Зелёный', 'API сервер доступен, все проверки пройдены'] },
          { en: ['Disconnected', 'Red', 'Cannot reach API server, check network/kubeconfig'], ru: ['Отключён', 'Красный', 'Нет связи с API сервером, проверьте сеть/kubeconfig'] },
          { en: ['Degraded', 'Yellow', 'High error rate or latency detected'], ru: ['Ухудшен', 'Жёлтый', 'Обнаружен высокий уровень ошибок или задержка'] },
        ],
      },
      { type: 'heading', content: { en: 'Namespace Management', ru: 'Управление Namespace' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'PEPA automatically discovers all namespaces in connected clusters', ru: 'PEPA автоматически обнаруживает все namespaces в подключённых кластерах' },
        { en: 'Deploy services to specific namespaces for environment isolation', ru: 'Разверните сервисы в определённые namespaces для изоляции окружений' },
        { en: 'View resource usage per namespace (CPU, memory, pods)', ru: 'Просмотр использования ресурсов по namespace (CPU, память, поды)' },
        { en: 'Create new namespaces directly from the cluster view', ru: 'Создавайте новые namespaces прямо из просмотра кластера' },
      ]},
      { type: 'heading', content: { en: 'Kubeconfig Best Practices', ru: 'Лучшие практики Kubeconfig' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Use service accounts with minimal required permissions (not admin)', ru: 'Используйте service accounts с минимально необходимыми правами (не admin)' },
        { en: 'Rotate kubeconfig certificates before they expire (typically 1 year)', ru: 'Обновляйте сертификаты kubeconfig до истечения срока (обычно 1 год)' },
        { en: 'Store kubeconfig securely — PEPA encrypts credentials at rest', ru: 'Храните kubeconfig безопасно — PEPA шифрует учётные данные' },
        { en: 'Use separate kubeconfig contexts for staging and production clusters', ru: 'Используйте отдельные контексты kubeconfig для staging и production кластеров' },
      ]},
    ],
  },
  {
    id: 'connections',
    title: { en: 'Connections & Integrations', ru: 'Подключения и интеграции' },
    blocks: [
      { type: 'text', content: {
        en: 'Manage all external connections from one place. Credentials are stored encrypted in the database using AES-256 encryption. Each connection includes a health check that is periodically verified.',
        ru: 'Управляйте всеми внешними подключениями из одного места. Учётные данные хранятся зашифрованными в базе данных с использованием шифрования AES-256. Каждое подключение включает проверку здоровья, которая периодически проверяется.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-connectiom.png', alt: { en: 'Connections and integrations', ru: 'Подключения и интеграции' }},
      { type: 'heading', content: { en: 'Connection Types', ru: 'Типы подключений' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Type', ru: 'Тип' }, { en: 'Description', ru: 'Описание' }, { en: 'Configuration', ru: 'Конфигурация' }],
        rows: [
          { en: ['Kubernetes', 'Cluster access', 'Kubeconfig file'], ru: ['Kubernetes', 'Доступ к кластеру', 'Файл Kubeconfig'] },
          { en: ['GitLab', 'Git + CI/CD', 'URL + Token'], ru: ['GitLab', 'Git + CI/CD', 'URL + Токен'] },
          { en: ['GitHub', 'Git + Actions', 'URL + Token'], ru: ['GitHub', 'Git + Actions', 'URL + Токен'] },
          { en: ['Bitbucket', 'Git + Pipelines', 'URL + Token'], ru: ['Bitbucket', 'Git + Pipelines', 'URL + Токен'] },
          { en: ['Jira', 'Issue tracking', 'URL + Token'], ru: ['Jira', 'Отслеживание задач', 'URL + Токен'] },
          { en: ['AI Provider', 'LLM access', 'API Key'], ru: ['AI Провайдер', 'Доступ к LLM', 'API Ключ'] },
          { en: ['Storage (S3)', 'S3/MinIO', 'Endpoint + Keys'], ru: ['Хранилище (S3)', 'S3/MinIO', 'Endpoint + Ключи'] },
          { en: ['Slack', 'Notifications', 'Webhook URL'], ru: ['Slack', 'Уведомления', 'Webhook URL'] },
        ],
      },
      { type: 'heading', content: { en: 'Connection Health Status', ru: 'Статус здоровья подключений' }},
      { type: 'text', content: {
        en: 'Each connection shows a status indicator: green (healthy), yellow (degraded), or red (unreachable). PEPA automatically tests connections every 60 seconds. You can also manually test a connection by clicking the Test button.',
        ru: 'Каждое подключение показывает индикатор: зелёный (исправен), жёлтый (ухудшен) или красный (недоступен). PEPA автоматически проверяет подключения каждые 60 секунд. Вы также можете вручную проверить подключение, нажав кнопку Test.',
      }},
      { type: 'heading', content: { en: 'Setting Up GitLab Connection', ru: 'Настройка подключения GitLab' }},
      { type: 'code', content: {
        en: '# 1. Generate a GitLab Personal Access Token\n# GitLab → Settings → Access Tokens → Create token\n# Required scopes: api, read_user, read_repository\n\n# 2. In PEPA: Connections → Add Connection → GitLab\n# URL: https://gitlab.com (or your self-hosted URL)\n# Token: paste the token\n\n# 3. Click Test Connection\n# Expected: green "Connected" status\n\n# 4. Save — PEPA will discover projects and repositories',
        ru: '# 1. Создайте GitLab Personal Access Token\n# GitLab → Settings → Access Tokens → Создать токен\n# Необходимые права: api, read_user, read_repository\n\n# 2. В PEPA: Подключения → Добавить подключение → GitLab\n# URL: https://gitlab.com (или ваш self-hosted URL)\n# Token: вставьте токен\n\n# 3. Нажмите Test Connection\n# Ожидается: зелёный статус "Connected"\n\n# 4. Сохраните — PEPA обнаружит проекты и репозитории',
      }},
      { type: 'heading', content: { en: 'Setting Up AI Provider', ru: 'Настройка AI Провайдера' }},
      { type: 'code', content: {
        en: '# Supported providers: OpenAI, Anthropic, local models\n\n# 1. Connections → Add Connection → AI Provider\n# Provider: OpenAI\n# API Key: sk-...\n# Model: gpt-4 (or gpt-3.5-turbo)\n\n# 2. Test Connection — should return green\n# 3. Save — the connection is applied to the AI Assistant automatically\n# Embedding model: text-embedding-ada-002\n# Chunk size: 1000\n# Top-K results: 5',
        ru: '# Поддерживаемые провайдеры: OpenAI, Anthropic, локальные модели\n\n# 1. Подключения → Добавить подключение → AI Провайдер\n# Provider: OpenAI\n# API Key: sk-...\n# Model: gpt-4 (или gpt-3.5-turbo)\n\n# 2. Test Connection — должен вернуть зелёный\n# 3. Сохраните — подключение автоматически применится к AI Ассистенту\n# Embedding model: text-embedding-ada-002\n# Chunk size: 1000\n# Top-K results: 5',
      }},
    ],
  },
  {
    id: 'pipelines',
    title: { en: 'CI/CD Pipelines', ru: 'CI/CD Пайплайны' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA provides built-in CI/CD pipeline management. Navigate to Pipelines to see all configured pipelines with run history, step-by-step logs, and the ability to re-run failed jobs.',
        ru: 'PEPA предоставляет встроенное управление CI/CD пайплайнами. Перейдите в Пайплайны для просмотра всех настроенных пайплайнов с историей запусков, пошаговыми логами и возможностью перезапуска упавших задач.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-pipelines-ci-cd.png', alt: { en: 'CI/CD Pipelines overview', ru: 'Обзор CI/CD пайплайнов' }},
      { type: 'heading', content: { en: 'Pipeline Structure', ru: 'Структура пайплайна' }},
      { type: 'code', content: {
        en: 'checkout → build → test → approval → deploy → notify\n   │        │       │        │          │        │\n   ▼        ▼       ▼        ▼          ▼        ▼\n  Git    Compile   Unit   Manual     K8s/Helm  Slack\n  pull   + lint   tests    gate      apply     alert',
        ru: 'checkout → build → test → approval → deploy → notify\n   │        │       │        │          │        │\n   ▼        ▼       ▼        ▼          ▼        ▼\n  Git    Компиляция Unit   Ручное     K8s/Helm  Slack\n  pull   + lint   тесты   gate       apply     alert',
      }},
      { type: 'heading', content: { en: 'Pipeline Step Types', ru: 'Типы шагов пайплайна' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Step', ru: 'Шаг' }, { en: 'Description', ru: 'Описание' }, { en: 'Configurable', ru: 'Настраиваемый' }],
        rows: [
          { en: ['checkout', 'Clone source code from Git repository', 'Yes — branch, tag, commit'], ru: ['checkout', 'Клонирование исходного кода из Git репозитория', 'Да — ветка, тег, коммит'] },
          { en: ['build', 'Compile code, build Docker image', 'Yes — build args, image tag'], ru: ['build', 'Компиляция кода, сборка Docker образа', 'Да — аргументы сборки, тег образа'] },
          { en: ['test', 'Run unit/integration tests', 'Yes — test command, timeout'], ru: ['test', 'Запуск unit/интеграционных тестов', 'Да — команда тестов, таймаут'] },
          { en: ['security_scan', 'Dependency audit + SAST + container scan', 'Yes — severity threshold'], ru: ['security_scan', 'Аудит зависимостей + SAST + сканирование контейнера', 'Да — порог критичности'] },
          { en: ['approval', 'Manual gate — requires human approval', 'Yes — approvers, timeout'], ru: ['approval', 'Ручной gate — требует подтверждения человека', 'Да — утверждающие, таймаут'] },
          { en: ['deploy', 'Deploy to Kubernetes via Helm/FluxCD', 'Yes — namespace, values, chart'], ru: ['deploy', 'Развёртывание в Kubernetes через Helm/FluxCD', 'Да — namespace, values, чарт'] },
          { en: ['notify', 'Send Slack/email notification', 'Yes — channel, message template'], ru: ['notify', 'Отправка уведомления в Slack/email', 'Да — канал, шаблон сообщения'] },
        ],
      },
      { type: 'heading', content: { en: 'Pre-built Blueprints', ru: 'Готовые шаблоны' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'CI/CD Pipeline — Full build-test-deploy flow with approval gate', ru: 'CI/CD Pipeline — Полный цикл build-test-deploy с approval gate' },
        { en: 'Security Scan — Dependency audit + SAST + container scan with reporting', ru: 'Security Scan — Аудит зависимостей + SAST + сканирование контейнеров с отчётностью' },
        { en: 'Rollback — Revert to previous version with health verification', ru: 'Rollback — Откат к предыдущей версии с проверкой здоровья' },
        { en: 'Hotfix — Emergency deploy bypassing approval gates', ru: 'Hotfix — Экстренный деплой в обход approval gate' },
      ]},
      { type: 'heading', content: { en: 'Pipeline Variables', ru: 'Переменные пайплайна' }},
      { type: 'text', content: {
        en: 'Pipelines support variables for dynamic configuration. Variables can be defined at the pipeline level or overridden per run. Sensitive values are encrypted using the vault integration.',
        ru: 'Пайплайны поддерживают переменные для динамической конфигурации. Переменные можно определить на уровне пайплайна или переопределить для каждого запуска. Конфиденциальные значения шифруются с использованием интеграции с vault.',
      }},
      { type: 'heading', content: { en: 'Viewing Pipeline Logs', ru: 'Просмотр логов пайплайна' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Navigate to Pipelines → select a pipeline → click a run', ru: 'Перейдите в Пайплайны → выберите пайплайн → нажмите на запуск' },
        { en: 'Each step shows expandable logs with timestamps', ru: 'Каждый шаг показывает раскрываемые логи с временными метками' },
        { en: 'Failed steps are highlighted in red with error details', ru: 'Упавшие шаги выделены красным с деталями ошибки' },
        { en: 'Use "Re-run" to retry failed steps without re-running the entire pipeline', ru: 'Используйте "Re-run" для повторного запуска упавших шагов без перезапуска всего пайплайна' },
      ]},
    ],
  },
  {
    id: 'gitops',
    title: { en: 'GitOps Workflows', ru: 'GitOps рабочие процессы' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA uses GitOps principles powered by FluxCD: Git is the single source of truth, FluxCD watches for changes, and Kubernetes applies the desired state automatically. This ensures all changes are auditable, reversible, and consistent.',
        ru: 'PEPA использует принципы GitOps на базе FluxCD: Git — единый источник истины, FluxCD следит за изменениями, Kubernetes автоматически применяет желаемое состояние. Это гарантирует что все изменения аудируемы, обратимы и консистентны.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-gitops.png', alt: { en: 'GitOps overview with drift detection', ru: 'Обзор GitOps с обнаружением дрейфа' }},
      { type: 'heading', content: { en: 'GitOps Flow', ru: 'Процесс GitOps' }},
      { type: 'code', content: {
        en: 'Git Repository (Source of Truth)\n    ↓\nFluxCD Agent (watches changes)\n    ↓\nKubernetes (applies desired state)\n    ↓\nVerification (health checks + rollback)',
        ru: 'Git Репозиторий (Источник истины)\n    ↓\nFluxCD Агент (следит за изменениями)\n    ↓\nKubernetes (применяет желаемое состояние)\n    ↓\nПроверка (health checks + откат)',
      }},
      { type: 'heading', content: { en: 'Repository Configuration', ru: 'Настройка репозитория' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to GitOps → Repositories → Add Repository', ru: 'Перейдите в GitOps → Репозитории → Добавить репозиторий' },
        { en: 'Select Git provider (GitLab, GitHub, Bitbucket, Gitea)', ru: 'Выберите Git провайдер (GitLab, GitHub, Bitbucket, Gitea)' },
        { en: 'Choose the repository and branch to watch', ru: 'Выберите репозиторий и ветку для наблюдения' },
        { en: 'Configure sync interval (default: 5 minutes)', ru: 'Настройте интервал синхронизации (по умолчанию: 5 минут)' },
        { en: 'Enable auto-heal to automatically reconcile drift', ru: 'Включите авто-исправление для автоматической синхронизации дрейфа' },
      ]},
      { type: 'heading', content: { en: 'Helm Releases', ru: 'Helm релизы' }},
      { type: 'text', content: {
        en: 'PEPA manages Helm releases through GitOps. Each service deployment creates a HelmRelease resource that FluxCD watches. You can view and manage releases from the GitOps page.',
        ru: 'PEPA управляет Helm релизами через GitOps. Каждое развёртывание сервиса создаёт ресурс HelmRelease, который FluxCD отслеживает. Вы можете просматривать и управлять релизами со страницы GitOps.',
      }},
      { type: 'heading', content: { en: 'Drift Detection', ru: 'Обнаружение дрейфа' }},
      { type: 'text', content: {
        en: 'Drift occurs when the live cluster state differs from the Git state. This can happen due to manual kubectl changes, other tools modifying resources, or infrastructure events.',
        ru: 'Дрейф возникает, когда реальное состояние кластера отличается от состояния в Git. Это может произойти из-за ручных изменений kubectl, других инструментов или событий инфраструктуры.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Review drift details to understand exactly what changed and when', ru: 'Просмотрите детали дрейфа для понимания что именно и когда изменилось' },
        { en: 'If the change was intentional, update the Git repository to match the desired state', ru: 'Если изменение было намеренным, обновите репозиторий Git для соответствия' },
        { en: 'If the change was accidental, trigger reconciliation from the GitOps page to restore Git state', ru: 'Если изменение было случайным, запустите синхронизацию со страницы GitOps' },
        { en: 'Enable auto-heal in repository settings to automatically reconcile drift', ru: 'Включите авто-исправление в настройках репозитория для автоматической синхронизации' },
        { en: 'Add admission webhooks to prevent manual changes to managed resources', ru: 'Добавьте admission webhooks для предотвращения ручных изменений управляемых ресурсов' },
      ]},
    ],
  },
  {
    id: 'workflows',
    title: { en: 'Workflow Engine', ru: 'Движок рабочих процессов' },
    blocks: [
      { type: 'text', content: {
        en: 'The Workflow Engine allows you to automate complex multi-step processes using a visual DAG (Directed Acyclic Graph) editor or YAML definitions. Workflows can include conditional logic, approval gates, parallel steps, and error handling.',
        ru: 'Движок рабочих процессов позволяет автоматизировать сложные многошаговые процессы с помощью визуального DAG (Направленный Ациклический Граф) редактора или YAML определений. Рабочие процессы могут включать условную логику, approval gate, параллельные шаги и обработку ошибок.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-workflow-designer.png', alt: { en: 'Visual Workflow DAG designer', ru: 'Визуальный DAG дизайнер рабочих процессов' }},
      { type: 'heading', content: { en: 'Creating a Workflow', ru: 'Создание рабочего процесса' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to Workflows → Create Workflow', ru: 'Перейдите в Рабочие процессы → Создать' },
        { en: 'Use the visual DAG editor to drag and drop steps, or switch to YAML mode', ru: 'Используйте визуальный DAG редактор для перетаскивания шагов или переключитесь в режим YAML' },
        { en: 'Define steps, conditions, approvals, and error handlers', ru: 'Определите шаги, условия, подтверждения и обработчики ошибок' },
        { en: 'Set triggers: manual, scheduled (cron), or event-based (webhook)', ru: 'Установите триггеры: ручной, по расписанию (cron) или по событию (webhook)' },
        { en: 'Save and activate the workflow', ru: 'Сохраните и активируйте рабочий процесс' },
      ]},
      { type: 'heading', content: { en: 'Workflow Templates', ru: 'Шаблоны рабочих процессов' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Template', ru: 'Шаблон' }, { en: 'Steps', ru: 'Шаги' }, { en: 'Use Case', ru: 'Сценарий' }],
        rows: [
          { en: ['CI/CD Pipeline', 'checkout → build → test → approval → deploy → notify', 'Standard deployment flow'], ru: ['CI/CD Pipeline', 'checkout → build → test → approval → deploy → notify', 'Стандартный процесс развёртывания'] },
          { en: ['Security Scan', 'checkout → dep_audit + sast + container_scan → report', 'Security compliance check'], ru: ['Security Scan', 'checkout → dep_audit + sast + container_scan → report', 'Проверка безопасности'] },
          { en: ['Entity Onboarding', 'validate → scorecard → notify_team', 'New service registration'], ru: ['Entity Onboarding', 'validate → scorecard → notify_team', 'Регистрация нового сервиса'] },
          { en: ['Rollback', 'get_previous → rollback → verify → notify', 'Emergency version revert'], ru: ['Rollback', 'get_previous → rollback → verify → notify', 'Экстренный откат версии'] },
        ],
      },
      { type: 'heading', content: { en: 'Workflow Triggers', ru: 'Триггеры рабочих процессов' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Trigger', ru: 'Триггер' }, { en: 'Description', ru: 'Описание' }, { en: 'Example', ru: 'Пример' }],
        rows: [
          { en: ['Manual', 'User clicks Run in the UI', 'On-demand deployment'], ru: ['Ручной', 'Пользователь нажимает Run в UI', 'Развёртывание по запросу'] },
          { en: ['Scheduled', 'Cron expression (e.g., 0 2 * * *)', 'Nightly security scan'], ru: ['По расписанию', 'Cron выражение (напр., 0 2 * * *)', 'Ночное сканирование безопасности'] },
          { en: ['Webhook', 'HTTP POST to workflow endpoint', 'Git push triggers CI'], ru: ['Webhook', 'HTTP POST на endpoint воркфлоу', 'Git push запускает CI'] },
          { en: ['Event', 'Platform event (service created, etc.)', 'Auto-scorecard on deploy'], ru: ['Событие', 'Событие платформы (создан сервис и т.д.)', 'Авто-оценка при деплое'] },
        ],
      },
      { type: 'heading', content: { en: 'Approval Gates', ru: 'Approval Gates' }},
      { type: 'text', content: {
        en: 'Approval gates pause workflow execution until a designated approver reviews and approves or rejects the step. This is commonly used before production deployments.',
        ru: 'Approval gates приостанавливают выполнение рабочего процесса до тех пор, пока назначенный утверждающий не рассмотрит и не одобрит или отклонит шаг. Это обычно используется перед развёртыванием в продакшен.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Configure approvers by user or role', ru: 'Настройте утверждающих по пользователю или роли' },
        { en: 'Set timeout — workflow fails if not approved within time limit', ru: 'Установите таймаут — рабочий процесс падает если не одобрен в срок' },
        { en: 'Approvers see pending approvals in their notification bell', ru: 'Утверждающие видят ожидающие подтверждения в колокольчике уведомлений' },
      ]},
    ],
  },
  {
    id: 'scorecards',
    title: { en: 'Scorecards', ru: 'Оценочные карты' },
    blocks: [
      { type: 'text', content: {
        en: 'Scorecards evaluate services against weighted rules to measure production readiness, quality, and compliance. They help teams understand what needs to be improved before a service can go to production.',
        ru: 'Оценочные карты оценивают сервисы по взвешенным правилам для измерения готовности к продакшену, качества и соответствия. Они помогают командам понять, что нужно улучшить перед выводом сервиса в продакшен.',
      }},
      { type: 'heading', content: { en: 'Score Levels', ru: 'Уровни оценки' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Level', ru: 'Уровень' }, { en: 'Threshold', ru: 'Порог' }, { en: 'Meaning', ru: 'Значение' }],
        rows: [
          { en: ['Bronze', '25%', 'Basic requirements met'], ru: ['Бронзовый', '25%', 'Базовые требования выполнены'] },
          { en: ['Silver', '50%', 'Ready for staging deployment'], ru: ['Серебряный', '50%', 'Готов к развёртыванию в staging'] },
          { en: ['Gold', '75%', 'Ready for production deployment'], ru: ['Золотой', '75%', 'Готов к развёртыванию в продакшен'] },
          { en: ['Platinum', '90%', 'Exceeds production standards'], ru: ['Платиновый', '90%', 'Превышает стандарты продакшена'] },
        ],
      },
      { type: 'heading', content: { en: 'Default Rules', ru: 'Правила по умолчанию' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Rule', ru: 'Правило' }, { en: 'Severity', ru: 'Критичность' }, { en: 'Weight', ru: 'Вес' }],
        rows: [
          { en: ['Health endpoint configured', 'Error', '20%'], ru: ['Health endpoint настроен', 'Ошибка', '20%'] },
          { en: ['Readiness endpoint configured', 'Error', '15%'], ru: ['Readiness endpoint настроен', 'Ошибка', '15%'] },
          { en: ['Owner team assigned', 'Warning', '15%'], ru: ['Команда назначена', 'Предупреждение', '15%'] },
          { en: ['Resource limits set', 'Warning', '15%'], ru: ['Лимиты ресурсов установлены', 'Предупреждение', '15%'] },
          { en: ['Replica count >= 2', 'Warning', '10%'], ru: ['Количество реплик >= 2', 'Предупреждение', '10%'] },
          { en: ['GitLab project linked', 'Info', '10%'], ru: ['Проект GitLab связан', 'Инфо', '10%'] },
          { en: ['Documentation exists', 'Info', '5%'], ru: ['Документация существует', 'Инфо', '5%'] },
          { en: ['Scorecard evaluation passes', 'Info', '10%'], ru: ['Оценка по карте пройдена', 'Инфо', '10%'] },
        ],
      },
      { type: 'heading', content: { en: 'Custom Rules', ru: 'Пользовательские правила' }},
      { type: 'text', content: {
        en: 'You can create custom scorecard rules to match your organization\'s requirements. Go to Scorecards → Create Rule and define the expression, severity, and weight.',
        ru: 'Вы можете создавать пользовательские правила оценочной карты для соответствия требованиям вашей организации. Перейдите в Оценочные карты → Создать правило и определите выражение, критичность и вес.',
      }},
    ],
  },
  {
    id: 'rbac',
    title: { en: 'RBAC — Roles & Permissions', ru: 'RBAC — Роли и права доступа' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA uses Role-Based Access Control (RBAC) to manage who can access what. Go to Settings → Users or Roles in the sidebar to manage access control. RBAC ensures teams only see and modify resources they own.',
        ru: 'PEPA использует Role-Based Access Control (RBAC) для управления доступом. Перейдите в Настройки → Пользователи или Роли в боковой панели для управления доступом. RBAC гарантирует что команды видят и изменяют только свои ресурсы.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-roles.png', alt: { en: 'RBAC roles and permissions management', ru: 'Управление ролями и правами доступа RBAC' }},
      { type: 'heading', content: { en: 'Permission Matrix', ru: 'Матрица прав доступа' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Resource', ru: 'Ресурс' }, { en: 'Admin', ru: 'Admin' }, { en: 'Developer', ru: 'Developer' }, { en: 'Viewer', ru: 'Viewer' }],
        rows: [
          { en: ['Services', 'Create/Read/Update/Delete', 'Create/Read/Update', 'Read'], ru: ['Сервисы', 'Создание/Чтение/Обновление/Удаление', 'Создание/Чтение/Обновление', 'Чтение'] },
          { en: ['Clusters', 'Full access', 'Read', 'Read'], ru: ['Кластеры', 'Полный доступ', 'Чтение', 'Чтение'] },
          { en: ['Pipelines', 'Full access', 'Create/Read/Update', 'Read'], ru: ['Пайплайны', 'Полный доступ', 'Создание/Чтение/Обновление', 'Чтение'] },
          { en: ['Workflows', 'Full access', 'Create/Read/Update', 'Read'], ru: ['Рабочие процессы', 'Полный доступ', 'Создание/Чтение/Обновление', 'Чтение'] },
          { en: ['Settings', 'Full access', 'Read own profile', 'No access'], ru: ['Настройки', 'Полный доступ', 'Чтение своего профиля', 'Нет доступа'] },
          { en: ['Users', 'Full access', 'No access', 'No access'], ru: ['Пользователи', 'Полный доступ', 'Нет доступа', 'Нет доступа'] },
        ],
      },
      { type: 'heading', content: { en: 'Creating Custom Roles', ru: 'Создание пользовательских ролей' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to Roles → Create Role', ru: 'Перейдите в Роли → Создать роль' },
        { en: 'Define name and description', ru: 'Укажите имя и описание' },
        { en: 'Select resource permissions (services, clusters, workflows, settings)', ru: 'Выберите права на ресурсы (сервисы, кластеры, рабочие процессы, настройки)' },
        { en: 'Choose permission level per resource: None, Read, Create/Read/Update, Full', ru: 'Выберите уровень прав для каждого ресурса: Нет, Чтение, Создание/Чтение/Обновление, Полный' },
        { en: 'Save and assign to users via Settings → Users', ru: 'Сохраните и назначьте пользователям через Настройки → Пользователи' },
      ]},
      { type: 'heading', content: { en: 'Teams', ru: 'Команды' }},
      { type: 'text', content: {
        en: 'Teams group users together for resource-level access control. A team can own services, and team members inherit access to those services. Go to Settings → Teams to manage.',
        ru: 'Команды группируют пользователей для контроля доступа на уровне ресурсов. Команда может владеть сервисами, и участники команды наследуют доступ к этим сервисам. Перейдите в Настройки → Команды для управления.',
      }},
    ],
  },
  {
    id: 'ai',
    title: { en: 'AI Assistant', ru: 'AI Ассистент' },
    blocks: [
      { type: 'text', content: {
        en: 'The AI Assistant uses RAG (Retrieval-Augmented Generation) with PGvector embeddings to provide context-aware answers about your platform. It understands your services, clusters, deployments, and configurations.',
        ru: 'AI Ассистент использует RAG (Retrieval-Augmented Generation) с PGvector эмбеддингами для предоставления контекстных ответов о вашей платформе. Он понимает ваши сервисы, кластеры, развёртывания и конфигурации.',
      }},
      { type: 'heading', content: { en: 'Setting Up AI', ru: 'Настройка AI' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to Connections → Add Connection → AI Provider to configure the AI provider', ru: 'Перейдите в Подключения → Добавить подключение → AI Провайдер для настройки AI провайдера' },
        { en: 'Select provider: OpenAI, Anthropic, or custom endpoint', ru: 'Выберите провайдера: OpenAI, Anthropic или custom endpoint' },
        { en: 'Enter API key and select model (e.g., gpt-4, claude-3-sonnet)', ru: 'Введите API ключ и выберите модель (например, gpt-4, claude-3-sonnet)' },
        { en: 'Configure RAG settings: embedding model, chunk size, top-K results', ru: 'Настройте параметры RAG: модель эмбеддингов, размер чанка, количество результатов' },
        { en: 'Click Test Connection to verify the AI provider is accessible', ru: 'Нажмите Test Connection для проверки доступности AI провайдера' },
      ]},
      { type: 'heading', content: { en: 'Example Queries', ru: 'Примеры запросов' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Query', ru: 'Запрос' }, { en: 'What it does', ru: 'Что делает' }],
        rows: [
          { en: ['"What services are unhealthy?"', 'Lists services with failed health checks'], ru: ['"Какие сервисы нездоровы?"', 'Список сервисов с проваленными проверками'] },
          { en: ['"Show deployment history for api-gateway"', 'Returns recent deployments for that service'], ru: ['"Покажи историю развёртываний api-gateway"', 'Возвращает последние развёртывания сервиса'] },
          { en: ['"Generate a Helm values file for a Node.js service"', 'Creates a template values.yaml'], ru: ['"Сгенерируй Helm values для Node.js сервиса"', 'Создаёт шаблон values.yaml'] },
          { en: ['"Which clusters are running Kubernetes 1.28?"', 'Filters clusters by version'], ru: ['"Какие кластеры работают на Kubernetes 1.28?"', 'Фильтрует кластеры по версии'] },
          { en: ['"Explain the GitOps drift on production-east"', 'Summarizes drift details'], ru: ['"Объясни дрейф GitOps на production-east"', 'Суммаризирует детали дрейфа'] },
        ],
      },
      { type: 'heading', content: { en: 'RAG Configuration', ru: 'Настройка RAG' }},
      { type: 'text', content: {
        en: 'RAG works by embedding your platform data (services, configs, docs) into vector embeddings stored in PGvector. When you ask a question, the AI retrieves the most relevant context before generating an answer.',
        ru: 'RAG работает путём преобразования данных вашей платформы (сервисы, конфигурации, документация) в векторные эмбеддинги, хранящиеся в PGvector. Когда вы задаёте вопрос, AI извлекает наиболее релевантный контекст перед генерацией ответа.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Embedding model: text-embedding-ada-002 (OpenAI) or equivalent', ru: 'Модель эмбеддингов: text-embedding-ada-002 (OpenAI) или эквивалент' },
        { en: 'Chunk size: 1000 tokens (adjust for more/less context per chunk)', ru: 'Размер чанка: 1000 токенов (настройте для большего/меньшего контекста на чанк)' },
        { en: 'Top-K results: 5 (number of relevant chunks retrieved per query)', ru: 'Top-K результатов: 5 (количество релевантных чанков, извлекаемых по запросу)' },
        { en: 'Similarity threshold: 0.7 (minimum similarity score to include)', ru: 'Порог сходства: 0.7 (минимальная оценка сходства для включения)' },
      ]},
    ],
  },
  {
    id: 'plugins',
    title: { en: 'Plugin System', ru: 'Система плагинов' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA has a modular plugin system that extends platform functionality. Plugins run as separate processes and communicate with the API server via gRPC. Built-in plugins are included with the platform, and custom plugins can be developed using the Go SDK.',
        ru: 'PEPA имеет модульную систему плагинов, расширяющую функциональность платформы. Плагины работают как отдельные процессы и общаются с API сервером через gRPC. Встроенные плагины включены в платформу, а пользовательские плагины могут быть разработаны с помощью Go SDK.',
      }},
      { type: 'image', content: { en: '', ru: '' }, src: '/screenshots/screenshot-marketplace.png', alt: { en: 'Plugin Marketplace catalog', ru: 'Каталог Marketplace плагинов' }},
      { type: 'heading', content: { en: 'Built-in Plugins', ru: 'Встроенные плагины' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Plugin', ru: 'Плагин' }, { en: 'Description', ru: 'Описание' }, { en: 'Status', ru: 'Статус' }],
        rows: [
          { en: ['Slack', 'Send notifications to Slack channels', 'Stable'], ru: ['Slack', 'Отправка уведомлений в каналы Slack', 'Стабильный'] },
          { en: ['ArgoCD', 'ArgoCD integration for GitOps', 'Stable'], ru: ['ArgoCD', 'Интеграция ArgoCD для GitOps', 'Стабильный'] },
          { en: ['GitHub', 'GitHub repository and Actions integration', 'Stable'], ru: ['GitHub', 'Интеграция репозитория и Actions GitHub', 'Стабильный'] },
          { en: ['GitLab', 'GitLab repository and CI/CD integration', 'Stable'], ru: ['GitLab', 'Интеграция репозитория и CI/CD GitLab', 'Стабильный'] },
          { en: ['Jira', 'Jira issue tracking integration', 'Stable'], ru: ['Jira', 'Интеграция отслеживания задач Jira', 'Стабильный'] },
          { en: ['Bitbucket', 'Bitbucket repository integration', 'Beta'], ru: ['Bitbucket', 'Интеграция репозитория Bitbucket', 'Бета'] },
          { en: ['Gitea', 'Gitea repository integration', 'Beta'], ru: ['Gitea', 'Интеграция репозитория Gitea', 'Бета'] },
          { en: ['FluxCD', 'FluxCD GitOps integration', 'Stable'], ru: ['FluxCD', 'Интеграция FluxCD GitOps', 'Стабильный'] },
        ],
      },
      { type: 'heading', content: { en: 'Installing Plugins', ru: 'Установка плагинов' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Go to Plugins or Marketplace', ru: 'Перейдите в Плагины или Маркетплейс' },
        { en: 'Browse available plugins and click Install', ru: 'Просмотрите доступные плагины и нажмите Установить' },
        { en: 'Configure connection settings (API keys, URLs, etc.)', ru: 'Настройте параметры подключения (API ключи, URL и т.д.)' },
        { en: 'Enable the plugin — it will start automatically', ru: 'Включите плагин — он запустится автоматически' },
        { en: 'Check plugin health on the Plugins page (green = healthy)', ru: 'Проверьте здоровье плагина на странице Плагины (зелёный = исправен)' },
      ]},
      { type: 'heading', content: { en: 'Plugin Configuration', ru: 'Настройка плагинов' }},
      { type: 'text', content: {
        en: 'Each plugin has its own configuration panel. Common settings include: API endpoint URL, authentication token/credentials, webhook URLs, and sync intervals. Configuration is stored encrypted in the database.',
        ru: 'Каждый плагин имеет свою панель настройки. Общие настройки включают: URL API endpoint, токен аутентификации/учётные данные, URL webhook и интервалы синхронизации. Конфигурация хранится зашифрованной в базе данных.',
      }},
      { type: 'heading', content: { en: 'Custom Plugin Development', ru: 'Разработка пользовательских плагинов' }},
      { type: 'text', content: {
        en: 'Create custom plugins using the PEPA Plugin SDK for Go. Plugins implement a simple interface with Name, Version, and Actions methods.',
        ru: 'Создавайте пользовательские плагины с помощью PEPA Plugin SDK для Go. Плагины реализуют простой интерфейс с методами Name, Version и Actions.',
      }},
      { type: 'code', content: {
        en: 'package main\n\nimport sdk "github.com/pepa/pepa/internal/plugin/sdk-go"\n\ntype MyPlugin struct{}\n\nfunc (p *MyPlugin) Name() string    { return "my-plugin" }\nfunc (p *MyPlugin) Version() string { return "0.1.0" }\nfunc (p *MyPlugin) Actions() []sdk.Action {\n    return []sdk.Action{\n        {\n            Name:        "greet",\n            Description: "Say hello",\n            Handler:     p.greetHandler,\n        },\n    }\n}\n\nfunc (p *MyPlugin) greetHandler(ctx sdk.Context) error {\n    ctx.Log("Hello from my-plugin!")\n    return nil\n}\n\nfunc main() {\n    sdk.Register(&MyPlugin{})\n    sdk.Serve()\n}',
        ru: 'package main\n\nimport sdk "github.com/pepa/pepa/internal/plugin/sdk-go"\n\ntype MyPlugin struct{}\n\nfunc (p *MyPlugin) Name() string    { return "my-plugin" }\nfunc (p *MyPlugin) Version() string { return "0.1.0" }\nfunc (p *MyPlugin) Actions() []sdk.Action {\n    return []sdk.Action{\n        {\n            Name:        "greet",\n            Description: "Say hello",\n            Handler:     p.greetHandler,\n        },\n    }\n}\n\nfunc (p *MyPlugin) greetHandler(ctx sdk.Context) error {\n    ctx.Log("Hello from my-plugin!")\n    return nil\n}\n\nfunc main() {\n    sdk.Register(&MyPlugin{})\n    sdk.Serve()\n}',
      }},
    ],
  },
  {
    id: 'troubleshooting',
    title: { en: 'Troubleshooting', ru: 'Устранение неполадок' },
    blocks: [
      { type: 'text', content: {
        en: 'This section covers the most common problems you may encounter when working with PEPA, their causes, and step-by-step solutions.',
        ru: 'Этот раздел описывает наиболее распространённые проблемы при работе с PEPA, их причины и пошаговые решения.',
      }},

      { type: 'heading', content: { en: '1. Cannot connect to Kubernetes cluster', ru: '1. Не удаётся подключиться к кластеру Kubernetes' }},
      { type: 'text', content: {
        en: 'Symptoms: Cluster shows "Disconnected" or "Error" status. Nodes and namespaces are not displayed.',
        ru: 'Симптомы: Кластер показывает статус "Отключён" или "Ошибка". Узлы и namespaces не отображаются.',
      }},
      { type: 'text', content: { en: 'Possible causes:', ru: 'Возможные причины:' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Invalid or expired kubeconfig file', ru: 'Невалидный или истёкший файл kubeconfig' },
        { en: 'Network connectivity issue between PEPA and cluster API server', ru: 'Проблема сетевой связности между PEPA и API сервером кластера' },
        { en: 'Insufficient RBAC permissions in the kubeconfig context', ru: 'Недостаточные RBAC права в контексте kubeconfig' },
        { en: 'Cluster API server certificate expired', ru: 'Истёк сертификат API сервера кластера' },
        { en: 'Firewall blocking traffic on port 6443', ru: 'Firewall блокирует трафик на порту 6443' },
      ]},
      { type: 'text', content: { en: 'Solutions:', ru: 'Решения:' }},
      { type: 'code', content: {
        en: '# 1. Verify kubeconfig is valid locally\nkubectl --kubeconfig=<your-file> get nodes\n\n# 2. Check network connectivity from the API container\ndocker exec pepa-api curl -k https://<cluster-endpoint>:6443/healthz\n\n# 3. Verify kubeconfig has cluster-admin or sufficient permissions\nkubectl --kubeconfig=<your-file> auth can-i get nodes\n\n# 4. Check if certificate is expired\nkubectl --kubeconfig=<your-file> config view --raw | \\\n  openssl x509 -noout -dates\n\n# 5. Re-upload the kubeconfig via Clusters → Add Cluster',
        ru: '# 1. Проверьте валидность kubeconfig локально\nkubectl --kubeconfig=<ваш-файл> get nodes\n\n# 2. Проверьте сетевую связность из контейнера API\ndocker exec pepa-api curl -k https://<endpoint-кластера>:6443/healthz\n\n# 3. Убедитесь что kubeconfig имеет cluster-admin или достаточные права\nkubectl --kubeconfig=<ваш-файл> auth can-i get nodes\n\n# 4. Проверьте срок действия сертификата\nkubectl --kubeconfig=<ваш-файл> config view --raw | \\\n  openssl x509 -noout -dates\n\n# 5. Перезагрузите kubeconfig через Кластеры → Добавить кластер',
      }},

      { type: 'heading', content: { en: '2. Frontend shows blank page or fails to load', ru: '2. Фронтенд показывает пустую страницу или не загружается' }},
      { type: 'text', content: {
        en: 'Symptoms: White/blank screen in browser, or "Page not found" error, or infinite loading.',
        ru: 'Симптомы: Белый экран в браузере, ошибка "Page not found", или бесконечная загрузка.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Clear browser cache completely (Cmd+Shift+R / Ctrl+Shift+R)', ru: 'Полностью очистите кэш браузера (Cmd+Shift+R / Ctrl+Shift+R)' },
        { en: 'Check API server is running: curl http://localhost:8088/healthz', ru: 'Проверьте работу API: curl http://localhost:8088/healthz' },
        { en: 'Check browser console for errors (F12 → Console tab)', ru: 'Проверьте консоль браузера на ошибки (F12 → вкладка Console)' },
        { en: 'Verify CORS_ORIGINS includes your frontend URL (http://localhost:3000)', ru: 'Убедитесь что CORS_ORIGINS содержит URL фронтенда (http://localhost:3000)' },
        { en: 'Check Docker container is running: docker compose ps', ru: 'Проверьте что Docker контейнер запущен: docker compose ps' },
        { en: 'Rebuild frontend after code changes: docker compose build --no-cache frontend', ru: 'Пересоберите фронтенд после изменений: docker compose build --no-cache frontend' },
        { en: 'Check Nginx logs if using reverse proxy: docker logs pepa-nginx', ru: 'Проверьте логи Nginx если используется reverse proxy: docker logs pepa-nginx' },
      ]},

      { type: 'heading', content: { en: '3. Pipeline fails at deploy step', ru: '3. Пайплайн падает на шаге deploy' }},
      { type: 'text', content: {
        en: 'Symptoms: Pipeline run shows red/failed status at the "deploy" step. Other steps (build, test) may succeed.',
        ru: 'Симптомы: Запуск пайплайна показывает красный/ошибочный статус на шаге "deploy". Другие шаги (build, test) могут завершаться успешно.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Check cluster connectivity in Connections page — cluster must be green/connected', ru: 'Проверьте связность кластера на странице Подключения — кластер должен быть зелёный/подключён' },
        { en: 'Verify target namespace exists: kubectl get namespaces', ru: 'Убедитесь что целевой namespace существует: kubectl get namespaces' },
        { en: 'Ensure FluxCD is installed and running in the target cluster', ru: 'Убедитесь что FluxCD установлен и запущен в целевом кластере' },
        { en: 'Check Helm chart is valid and accessible from the configured repository', ru: 'Проверьте что Helm чарт валиден и доступен из настроенного репозитория' },
        { en: 'Review pipeline step logs for specific Kubernetes error messages', ru: 'Просмотрите логи шага пайплайна для конкретных сообщений об ошибках Kubernetes' },
        { en: 'Verify service account has permissions to deploy to the target namespace', ru: 'Убедитесь что service account имеет права на деплой в целевой namespace' },
      ]},

      { type: 'heading', content: { en: '4. AI Assistant returns errors or no response', ru: '4. AI Ассистент возвращает ошибки или не отвечает' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Verify AI provider API key is configured in Connections → AI Provider', ru: 'Убедитесь что API ключ AI провайдера настроен в Подключения → AI Провайдер' },
        { en: 'Check the AI provider is reachable from the API server container', ru: 'Проверьте доступность AI провайдера из контейнера API сервера' },
        { en: 'Ensure the AI provider account has sufficient credits/quota remaining', ru: 'Убедитесь что у аккаунта AI провайдера достаточно кредитов/квоты' },
        { en: 'Check API server logs: docker logs pepa-api | grep -i ai', ru: 'Проверьте логи API: docker logs pepa-api | grep -i ai' },
        { en: 'Verify the model name is correct (e.g., gpt-4, claude-3-sonnet)', ru: 'Убедитесь что имя модели корректно (например, gpt-4, claude-3-sonnet)' },
        { en: 'If using RAG, check PGvector extension is installed in PostgreSQL', ru: 'При использовании RAG, проверьте что расширение PGvector установлено в PostgreSQL' },
      ]},

      { type: 'heading', content: { en: '5. Database connection refused', ru: '5. Ошибка подключения к базе данных' }},
      { type: 'text', content: {
        en: 'Symptoms: API returns HTTP 500 errors. Logs show "connection refused" or "too many clients".',
        ru: 'Симптомы: API возвращает ошибки HTTP 500. Логи показывают "connection refused" или "too many clients".',
      }},
      { type: 'code', content: {
        en: '# 1. Check PostgreSQL container is running\ndocker compose ps postgres\n\n# 2. Verify connection parameters\ndocker exec pepa-api env | grep POSTGRES\n\n# 3. Test connection manually\ndocker exec pepa-postgres psql -U pepa -d pepa -c "SELECT 1"\n\n# 4. Check active connections (if "too many clients")\ndocker exec pepa-postgres psql -U pepa -d pepa -c \\\n  "SELECT count(*) FROM pg_stat_activity"\n\n# 5. Run pending migrations\ndocker exec pepa-postgres psql -U pepa -d pepa \\\n  -f /docker-entrypoint-initdb.d/01-init.sql\n\n# 6. Restart PostgreSQL if needed\ndocker compose restart postgres',
        ru: '# 1. Проверьте что контейнер PostgreSQL запущен\ndocker compose ps postgres\n\n# 2. Проверьте параметры подключения\ndocker exec pepa-api env | grep POSTGRES\n\n# 3. Проверьте подключение вручную\ndocker exec pepa-postgres psql -U pepa -d pepa -c "SELECT 1"\n\n# 4. Проверьте активные подключения (если "too many clients")\ndocker exec pepa-postgres psql -U pepa -d pepa -c \\\n  "SELECT count(*) FROM pg_stat_activity"\n\n# 5. Запустите ожидающие миграции\ndocker exec pepa-postgres psql -U pepa -d pepa \\\n  -f /docker-entrypoint-initdb.d/01-init.sql\n\n# 6. Перезапустите PostgreSQL при необходимости\ndocker compose restart postgres',
      }},

      { type: 'heading', content: { en: '6. Plugin fails to load or shows error', ru: '6. Плагин не загружается или показывает ошибку' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Check plugin binary exists in the plugins/ directory', ru: 'Проверьте наличие бинарного файла плагина в директории plugins/' },
        { en: 'Verify the plugin is compiled for the correct architecture (amd64/arm64)', ru: 'Убедитесь что плагин скомпилирован для правильной архитектуры (amd64/arm64)' },
        { en: 'Check API server logs for plugin load errors: docker logs pepa-api | grep plugin', ru: 'Проверьте логи API на ошибки загрузки: docker logs pepa-api | grep plugin' },
        { en: 'Ensure the plugin binary has execute permissions: chmod +x plugins/my-plugin', ru: 'Убедитесь что бинарный файл плагина имеет права на выполнение: chmod +x plugins/my-plugin' },
        { en: 'Reinstall the plugin from the Marketplace', ru: 'Переустановите плагин из Маркетплейса' },
        { en: 'Check PLUGIN_DIR environment variable points to the correct directory', ru: 'Убедитесь что переменная PLUGIN_DIR указывает на правильную директорию' },
      ]},

      { type: 'heading', content: { en: '7. GitOps drift detected', ru: '7. Обнаружен дрейф GitOps' }},
      { type: 'text', content: {
        en: 'Symptoms: Drift Detection page shows differences between Git state and live cluster state. Resources were changed manually or by another tool.',
        ru: 'Симптомы: Страница обнаружения дрейфа показывает различия между состоянием в Git и реальным состоянием кластера. Ресурсы были изменены вручную или другим инструментом.',
      }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Review drift details to understand exactly what changed and when', ru: 'Просмотрите детали дрейфа для понимания что именно и когда изменилось' },
        { en: 'If the change was intentional, update the Git repository to match the desired state', ru: 'Если изменение было намеренным, обновите репозиторий Git для соответствия' },
        { en: 'If the change was accidental, trigger reconciliation from the GitOps page to restore Git state', ru: 'Если изменение было случайным, запустите синхронизацию со страницы GitOps' },
        { en: 'Enable auto-heal in repository settings to automatically reconcile drift', ru: 'Включите авто-исправление в настройках репозитория для автоматической синхронизации' },
        { en: 'Add admission webhooks to prevent manual changes to managed resources', ru: 'Добавьте admission webhooks для предотвращения ручных изменений управляемых ресурсов' },
      ]},

      { type: 'heading', content: { en: '8. Redis connection issues', ru: '8. Проблемы с подключением к Redis' }},
      { type: 'text', content: {
        en: 'Symptoms: Worker fails to process jobs. Queue operations time out. API logs show Redis connection errors.',
        ru: 'Симптомы: Воркер не обрабатывает задачи. Операции очереди превышают таймаут. Логи API показывают ошибки подключения к Redis.',
      }},
      { type: 'code', content: {
        en: '# Check Redis container status\ndocker compose ps redis\n\n# Test Redis connection\ndocker exec pepa-redis redis-cli -a pepa_dev ping\n# Expected: PONG\n\n# Check Redis memory usage\ndocker exec pepa-redis redis-cli -a pepa_dev info memory\n\n# Check queue depth\ndocker exec pepa-redis redis-cli -a pepa_dev llen pepa:jobs\n\n# Restart Redis if needed\ndocker compose restart redis',
        ru: '# Проверьте статус контейнера Redis\ndocker compose ps redis\n\n# Проверьте подключение к Redis\ndocker exec pepa-redis redis-cli -a pepa_dev ping\n# Ожидается: PONG\n\n# Проверьте использование памяти Redis\ndocker exec pepa-redis redis-cli -a pepa_dev info memory\n\n# Проверьте глубину очереди\ndocker exec pepa-redis redis-cli -a pepa_dev llen pepa:jobs\n\n# Перезапустите Redis при необходимости\ndocker compose restart redis',
      }},

      { type: 'heading', content: { en: '9. Docker build fails', ru: '9. Ошибка сборки Docker' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Clear Docker build cache: docker builder prune -a', ru: 'Очистите кэш сборок Docker: docker builder prune -a' },
        { en: 'Check available disk space: df -h (Docker needs at least 5GB free)', ru: 'Проверьте свободное место на диске: df -h (Docker нужно минимум 5ГБ свободного места)' },
        { en: 'Verify Docker daemon is running and has enough memory allocated', ru: 'Убедитесь что Docker daemon запущен и имеет достаточно выделенной памяти' },
        { en: 'Check for port conflicts: lsof -i :3000 and lsof -i :8088', ru: 'Проверьте конфликты портов: lsof -i :3000 и lsof -i :8088' },
        { en: 'Rebuild with --no-cache flag: docker compose build --no-cache frontend', ru: 'Пересоберите с флагом --no-cache: docker compose build --no-cache frontend' },
      ]},

      { type: 'heading', content: { en: '10. Login / Authentication issues', ru: '10. Проблемы со входом / аутентификацией' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'In development mode (DEV_MODE=true), authentication is bypassed — no login required', ru: 'В режиме разработки (DEV_MODE=true) аутентификация пропускается — вход не требуется' },
        { en: 'In production, ensure JWT_SECRET is set to a strong random value (min 32 chars)', ru: 'В продакшене убедитесь что JWT_SECRET установлен в сильное случайное значение (мин. 32 символа)' },
        { en: 'If locked out, use CLI to reset: ./bin/pepa user reset-password --email admin@pepa.dev', ru: 'При блокировке используйте CLI: ./bin/pepa user reset-password --email admin@pepa.dev' },
        { en: 'Check user account is active: Settings → Users → verify Status is "Active"', ru: 'Убедитесь что аккаунт активен: Settings → Users → проверьте Status = "Active"' },
        { en: 'Clear browser cookies for localhost if JWT token is stale', ru: 'Очистите cookies браузера для localhost если JWT токен устарел' },
      ]},

      { type: 'heading', content: { en: '11. Service deployment hangs or times out', ru: '11. Развёртывание сервиса зависает или превышает таймаут' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Check if the target cluster has sufficient resources (CPU, memory, disk)', ru: 'Проверьте что целевой кластер имеет достаточно ресурсов (CPU, память, диск)' },
        { en: 'Verify the container image exists and is pullable by the cluster nodes', ru: 'Убедитесь что образ контейнера существует и доступен для загрузки узлами кластера' },
        { en: 'Check image pull secrets are configured if using a private registry', ru: 'Проверьте что image pull secrets настроены для приватного реестра' },
        { en: 'Review Kubernetes events: kubectl get events --sort-by=.lastTimestamp', ru: 'Просмотрите события Kubernetes: kubectl get events --sort-by=.lastTimestamp' },
        { en: 'Increase deployment timeout in Settings if builds are legitimately slow', ru: 'Увеличьте таймаут развёртывания в Settings если сборки легитимно медленные' },
      ]},

      { type: 'heading', content: { en: '12. MinIO / S3 storage errors', ru: '12. Ошибки хранилища MinIO / S3' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Verify MinIO container is running: docker compose ps minio', ru: 'Убедитесь что контейнер MinIO запущен: docker compose ps minio' },
        { en: 'Check S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY environment variables', ru: 'Проверьте переменные S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY' },
        { en: 'Verify buckets exist: access MinIO console at http://localhost:9001', ru: 'Убедитесь что бакеты существуют: откройте консоль MinIO на http://localhost:9001' },
        { en: 'Check disk space on the MinIO volume', ru: 'Проверьте свободное место на volume MinIO' },
        { en: 'Test connectivity: docker exec pepa-api curl http://minio:9000/minio/health/live', ru: 'Проверьте связность: docker exec pepa-api curl http://minio:9000/minio/health/live' },
      ]},

      { type: 'heading', content: { en: '13. Nginx reverse proxy errors (502/504)', ru: '13. Ошибки Nginx reverse proxy (502/504)' }},
      { type: 'text', content: {
        en: 'Symptoms: Browser shows 502 Bad Gateway or 504 Gateway Timeout. Nginx logs show upstream connection refused or timed out.',
        ru: 'Симптомы: Браузер показывает 502 Bad Gateway или 504 Gateway Timeout. Логи Nginx показывают upstream connection refused или timed out.',
      }},
      { type: 'code', content: {
        en: '# Check Nginx logs for details\ndocker logs pepa-nginx --tail 50\n\n# Verify frontend and API containers are running\ndocker compose ps frontend api\n\n# Check if ports are correct in nginx.conf\ndocker exec pepa-nginx cat /etc/nginx/conf.d/default.conf | grep proxy_pass\n\n# Restart Nginx\ndocker compose restart nginx\n\n# If 504, increase proxy timeout in nginx.conf:\n# proxy_read_timeout 120s;\n# proxy_connect_timeout 10s;',
        ru: '# Проверьте логи Nginx для деталей\ndocker logs pepa-nginx --tail 50\n\n# Убедитесь что контейнеры frontend и API запущены\ndocker compose ps frontend api\n\n# Проверьте правильность портов в nginx.conf\ndocker exec pepa-nginx cat /etc/nginx/conf.d/default.conf | grep proxy_pass\n\n# Перезапустите Nginx\ndocker compose restart nginx\n\n# Если 504, увеличьте таймаут proxy в nginx.conf:\n# proxy_read_timeout 120s;\n# proxy_connect_timeout 10s;',
      }},

      { type: 'heading', content: { en: '14. Port already in use', ru: '14. Порт уже занят' }},
      { type: 'text', content: {
        en: 'Symptoms: Docker compose fails to start with "Bind for 0.0.0.0:3000 failed: port is already allocated" or similar.',
        ru: 'Симптомы: Docker compose не может запуститься с ошибкой "Bind for 0.0.0.0:3000 failed: port is already allocated" или подобной.',
      }},
      { type: 'code', content: {
        en: '# Find what is using the port\nlsof -i :3000\nlsof -i :8088\nlsof -i :5432\n\n# Kill the process if safe\nkill -9 <PID>\n\n# Or change PEPA ports in .env:\n# FRONTEND_PORT=3001\n# API_PORT=8089\n# POSTGRES_PORT=5433',
        ru: '# Найдите что занимает порт\nlsof -i :3000\nlsof -i :8088\nlsof -i :5432\n\n# Заверните процесс если безопасно\nkill -9 <PID>\n\n# Или измените порты PEPA в .env:\n# FRONTEND_PORT=3001\n# API_PORT=8089\n# POSTGRES_PORT=5433',
      }},

      { type: 'heading', content: { en: '15. Database migration errors', ru: '15. Ошибки миграций базы данных' }},
      { type: 'text', content: {
        en: 'Symptoms: API fails to start with migration errors. Tables or columns are missing. Schema conflicts after upgrade.',
        ru: 'Симптомы: API не запускается с ошибками миграции. Таблицы или столбцы отсутствуют. Конфликты схемы после обновления.',
      }},
      { type: 'code', content: {
        en: '# Check which migrations have been applied\ndocker exec pepa-postgres psql -U pepa -d pepa -c \\\n  "SELECT * FROM schema_migrations ORDER BY version"\n\n# Manually apply a specific migration\ndocker cp migrations/017_auth.sql pepa-postgres:/tmp/\ndocker exec pepa-postgres psql -U pepa -d pepa -f /tmp/017_auth.sql\n\n# Reset database (WARNING: destroys all data)\ndocker compose down -v\ndocker compose up -d postgres\n# Wait for init, then start other services',
        ru: '# Проверьте какие миграции были применены\ndocker exec pepa-postgres psql -U pepa -d pepa -c \\\n  "SELECT * FROM schema_migrations ORDER BY version"\n\n# Вручную примените конкретную миграцию\ndocker cp migrations/017_auth.sql pepa-postgres:/tmp/\ndocker exec pepa-postgres psql -U pepa -d pepa -f /tmp/017_auth.sql\n\n# Сброс базы данных (ВНИМАНИЕ: уничтожает все данные)\ndocker compose down -v\ndocker compose up -d postgres\n# Подождите инициализации, затем запустите другие сервисы',
      }},

      { type: 'heading', content: { en: '16. Worker not processing jobs', ru: '16. Воркер не обрабатывает задачи' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Check worker container is running: docker compose ps worker', ru: 'Убедитесь что контейнер воркера запущен: docker compose ps worker' },
        { en: 'Check worker logs: docker logs pepa-worker', ru: 'Проверьте логи воркера: docker logs pepa-worker' },
        { en: 'Verify Redis connection (worker uses Redis for job queue)', ru: 'Проверьте подключение к Redis (воркер использует Redis для очереди задач)' },
        { en: 'Check queue depth: docker exec pepa-redis redis-cli -a pepa_dev llen pepa:jobs', ru: 'Проверьте глубину очереди: docker exec pepa-redis redis-cli -a pepa_dev llen pepa:jobs' },
        { en: 'Restart worker: docker compose restart worker', ru: 'Перезапустите воркер: docker compose restart worker' },
      ]},
    ],
  },
  {
    id: 'faq',
    title: { en: 'FAQ', ru: 'Часто задаваемые вопросы' },
    blocks: [
      { type: 'heading', content: { en: 'How do I reset the admin password?', ru: 'Как сбросить пароль администратора?' }},
      { type: 'code', content: {
        en: './bin/pepa user reset-password --email admin@pepa.dev --password newpass',
        ru: './bin/pepa user reset-password --email admin@pepa.dev --password newpass',
      }},
      { type: 'heading', content: { en: 'How do I add a new environment?', ru: 'Как добавить новое окружение?' }},
      { type: 'text', content: {
        en: 'Settings → Environments → Create Environment. Specify name, variables, and constraints.',
        ru: 'Настройки → Окружения → Создать окружение. Укажите имя, переменные и ограничения.',
      }},
      { type: 'heading', content: { en: 'Can I import existing Helm charts?', ru: 'Можно ли импортировать существующие Helm чарты?' }},
      { type: 'text', content: {
        en: 'Yes — use the Helm Import template when creating a service, or add a Helm repository at Helm Repos.',
        ru: 'Да — используйте шаблон Helm Import при создании сервиса, или добавьте Helm репозиторий в Helm Репозитории.',
      }},
      { type: 'heading', content: { en: 'How do I enable production mode?', ru: 'Как включить продакшен режим?' }},
      { type: 'code', content: {
        en: 'SERVER_ENV=production\nDEV_MODE=false\nJWT_SECRET=your-secret-key',
        ru: 'SERVER_ENV=production\nDEV_MODE=false\nJWT_SECRET=your-secret-key',
      }},
      { type: 'heading', content: { en: 'How do I backup the database?', ru: 'Как сделать резервную копию базы данных?' }},
      { type: 'code', content: {
        en: 'docker compose exec postgres pg_dump -U pepa pepa > backup_$(date +%Y%m%d).sql',
        ru: 'docker compose exec postgres pg_dump -U pepa pepa > backup_$(date +%Y%m%d).sql',
      }},
      { type: 'heading', content: { en: 'How do I scale the API server?', ru: 'Как масштабировать API сервер?' }},
      { type: 'text', content: {
        en: 'The API server is stateless. Run multiple instances behind a load balancer:',
        ru: 'API сервер не имеет состояния. Запустите несколько экземпляров за балансировщиком нагрузки:',
      }},
      { type: 'code', content: {
        en: 'docker compose -f docker-compose.prod.yml up --scale api=3 -d',
        ru: 'docker compose -f docker-compose.prod.yml up --scale api=3 -d',
      }},
      { type: 'heading', content: { en: 'How do I restore a database backup?', ru: 'Как восстановить базу данных из резервной копии?' }},
      { type: 'code', content: {
        en: 'docker compose exec -T postgres psql -U pepa pepa < backup_20250101.sql',
        ru: 'docker compose exec -T postgres psql -U pepa pepa < backup_20250101.sql',
      }},
      { type: 'heading', content: { en: 'How do I update PEPA to the latest version?', ru: 'Как обновить PEPA до последней версии?' }},
      { type: 'code', content: {
        en: '# 1. Pull latest changes\ngit pull origin main\n\n# 2. Rebuild and restart\ndocker compose -f deployments/compose/docker-compose.yml build --no-cache\ndocker compose -f deployments/compose/docker-compose.yml up -d\n\n# 3. Run any new migrations\ndocker compose exec api /app/bin/pepa migrate',
        ru: '# 1. Вытяните последние изменения\ngit pull origin main\n\n# 2. Пересоберите и перезапустите\ndocker compose -f deployments/compose/docker-compose.yml build --no-cache\ndocker compose -f deployments/compose/docker-compose.yml up -d\n\n# 3. Запустите новые миграции\ndocker compose exec api /app/bin/pepa migrate',
      }},
      { type: 'heading', content: { en: 'Can I use external PostgreSQL instead of Docker?', ru: 'Можно ли использовать внешний PostgreSQL вместо Docker?' }},
      { type: 'text', content: {
        en: 'Yes. Set POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB in .env to point to your external database. Ensure PGvector extension is available.',
        ru: 'Да. Установите POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB в .env для указания на вашу внешнюю базу данных. Убедитесь что расширение PGvector доступно.',
      }},
      { type: 'heading', content: { en: 'How do I configure TLS/SSL?', ru: 'Как настроить TLS/SSL?' }},
      { type: 'text', content: {
        en: 'Use the nginx-ssl Docker Compose profile. Place your certificate and key in deployments/compose/nginx-ssl/certs/ and update the nginx.conf accordingly.',
        ru: 'Используйте профиль nginx-ssl в Docker Compose. Поместите сертификат и ключ в deployments/compose/nginx-ssl/certs/ и обновите nginx.conf соответственно.',
      }},
    ],
  },
  {
    id: 'env-variables',
    title: { en: 'Environment Variables', ru: 'Переменные окружения' },
    blocks: [
      { type: 'text', content: {
        en: 'PEPA is configured via environment variables. Copy .env.example to .env and adjust values as needed. Below are the key variables grouped by category.',
        ru: 'PEPA настраивается через переменные окружения. Скопируйте .env.example в .env и измените значения при необходимости. Ниже приведены основные переменные, сгруппированные по категориям.',
      }},
      { type: 'heading', content: { en: 'Core Settings', ru: 'Основные настройки' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Variable', ru: 'Переменная' }, { en: 'Default', ru: 'По умолчанию' }, { en: 'Description', ru: 'Описание' }],
        rows: [
          { en: ['SERVER_ENV', 'development', 'Environment: development | production'], ru: ['SERVER_ENV', 'development', 'Окружение: development | production'] },
          { en: ['DEV_MODE', 'true', 'Skip authentication (dev only)'], ru: ['DEV_MODE', 'true', 'Пропустить аутентификацию (только для разработки)'] },
          { en: ['API_PORT', '8080', 'API server port'], ru: ['API_PORT', '8080', 'Порт API сервера'] },
          { en: ['FRONTEND_PORT', '3000', 'Frontend dev server port'], ru: ['FRONTEND_PORT', '3000', 'Порт dev сервера фронтенда'] },
          { en: ['LOG_LEVEL', 'info', 'Log level: debug | info | warn | error'], ru: ['LOG_LEVEL', 'info', 'Уровень логирования: debug | info | warn | error'] },
        ],
      },
      { type: 'heading', content: { en: 'Database', ru: 'База данных' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Variable', ru: 'Переменная' }, { en: 'Default', ru: 'По умолчанию' }, { en: 'Description', ru: 'Описание' }],
        rows: [
          { en: ['POSTGRES_HOST', 'localhost', 'PostgreSQL host'], ru: ['POSTGRES_HOST', 'localhost', 'Хост PostgreSQL'] },
          { en: ['POSTGRES_PORT', '5432', 'PostgreSQL port'], ru: ['POSTGRES_PORT', '5432', 'Порт PostgreSQL'] },
          { en: ['POSTGRES_USER', 'pepa', 'Database user'], ru: ['POSTGRES_USER', 'pepa', 'Пользователь базы данных'] },
          { en: ['POSTGRES_PASSWORD', 'pepa_dev', 'Database password'], ru: ['POSTGRES_PASSWORD', 'pepa_dev', 'Пароль базы данных'] },
          { en: ['POSTGRES_DB', 'pepa', 'Database name'], ru: ['POSTGRES_DB', 'pepa', 'Имя базы данных'] },
        ],
      },
      { type: 'heading', content: { en: 'Redis & Storage', ru: 'Redis и хранилище' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Variable', ru: 'Переменная' }, { en: 'Default', ru: 'По умолчанию' }, { en: 'Description', ru: 'Описание' }],
        rows: [
          { en: ['REDIS_HOST', 'localhost', 'Redis host'], ru: ['REDIS_HOST', 'localhost', 'Хост Redis'] },
          { en: ['REDIS_PORT', '6379', 'Redis port'], ru: ['REDIS_PORT', '6379', 'Порт Redis'] },
          { en: ['S3_ENDPOINT', 'http://localhost:9000', 'S3/MinIO endpoint'], ru: ['S3_ENDPOINT', 'http://localhost:9000', 'Endpoint S3/MinIO'] },
          { en: ['S3_ACCESS_KEY', 'minioadmin', 'S3 access key'], ru: ['S3_ACCESS_KEY', 'minioadmin', 'Ключ доступа S3'] },
          { en: ['S3_SECRET_KEY', 'minioadmin', 'S3 secret key'], ru: ['S3_SECRET_KEY', 'minioadmin', 'Секретный ключ S3'] },
        ],
      },
      { type: 'heading', content: { en: 'Security', ru: 'Безопасность' }},
      { type: 'table', content: { en: '', ru: '' },
        headers: [{ en: 'Variable', ru: 'Переменная' }, { en: 'Default', ru: 'По умолчанию' }, { en: 'Description', ru: 'Описание' }],
        rows: [
          { en: ['JWT_SECRET', '(auto-generated)', 'JWT signing secret (required in production)'], ru: ['JWT_SECRET', '(авто-генерация)', 'Секрет подписи JWT (обязателен в продакшене)'] },
          { en: ['CORS_ORIGINS', 'http://localhost:3000', 'Allowed CORS origins'], ru: ['CORS_ORIGINS', 'http://localhost:3000', 'Разрешённые CORS origins'] },
          { en: ['ENCRYPTION_KEY', '(auto-generated)', 'AES-256 key for credential encryption'], ru: ['ENCRYPTION_KEY', '(авто-генерация)', 'Ключ AES-256 для шифрования учётных данных'] },
        ],
      },
    ],
  },
  {
    id: 'production',
    title: { en: 'Production Deployment', ru: 'Продакшен развёртывание' },
    blocks: [
      { type: 'text', content: {
        en: 'This guide covers deploying PEPA in production with TLS, proper security settings, and high availability.',
        ru: 'Это руководство описывает развёртывание PEPA в продакшене с TLS, правильными настройками безопасности и высокой доступностью.',
      }},
      { type: 'heading', content: { en: 'Production Checklist', ru: 'Чек-лист продакшена' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Set DEV_MODE=false to enable authentication', ru: 'Установите DEV_MODE=false для включения аутентификации' },
        { en: 'Set JWT_SECRET to a strong random value (min 32 chars)', ru: 'Установите JWT_SECRET в сильное случайное значение (мин. 32 символа)' },
        { en: 'Change default database passwords (POSTGRES_PASSWORD)', ru: 'Измените пароли базы данных по умолчанию (POSTGRES_PASSWORD)' },
        { en: 'Configure TLS certificates for Nginx', ru: 'Настройте TLS сертификки для Nginx' },
        { en: 'Set CORS_ORIGINS to your production domain', ru: 'Установите CORS_ORIGINS на ваш продакшен домен' },
        { en: 'Enable database backups (cron job or managed service)', ru: 'Включите резервное копирование базы данных (cron или управляемый сервис)' },
        { en: 'Configure monitoring and alerting', ru: 'Настройте мониторинг и оповещения' },
        { en: 'Set LOG_LEVEL=warn for production (reduce noise)', ru: 'Установите LOG_LEVEL=warn для продакшена (меньше шума)' },
      ]},
      { type: 'heading', content: { en: 'Docker Compose Production', ru: 'Docker Compose для продакшена' }},
      { type: 'code', content: {
        en: '# Use the production compose file\ndocker compose -f deployments/compose/docker-compose.prod.yml up -d\n\n# This includes:\n# - Nginx with TLS termination\n# - Frontend (Next.js standalone)\n# - API server\n# - Worker\n# - PostgreSQL 18 + PGvector\n# - Redis\n# - MinIO (S3)\n# - Prometheus + Grafana (monitoring)',
        ru: '# Используйте продакшен compose файл\ndocker compose -f deployments/compose/docker-compose.prod.yml up -d\n\n# Это включает:\n# - Nginx с TLS терминацией\n# - Frontend (Next.js standalone)\n# - API сервер\n# - Worker\n# - PostgreSQL 18 + PGvector\n# - Redis\n# - MinIO (S3)\n# - Prometheus + Grafana (мониторинг)',
      }},
      { type: 'heading', content: { en: 'Helm Chart (Kubernetes)', ru: 'Helm чарт (Kubernetes)' }},
      { type: 'code', content: {
        en: '# Install via Helm\nhelm install pepa deployments/helm/pepa/ \\\n  --namespace pepa --create-namespace \\\n  --set global.domain=pepa.example.com \\\n  --set ingress.enabled=true \\\n  --set ingress.tls.enabled=true',
        ru: '# Установка через Helm\nhelm install pepa deployments/helm/pepa/ \\\n  --namespace pepa --create-namespace \\\n  --set global.domain=pepa.example.com \\\n  --set ingress.enabled=true \\\n  --set ingress.tls.enabled=true',
      }},
    ],
  },
  {
    id: 'backup',
    title: { en: 'Backup & Recovery', ru: 'Резервное копирование и восстановление' },
    blocks: [
      { type: 'text', content: {
        en: 'Regular backups are essential for production deployments. PEPA stores data in PostgreSQL, MinIO (S3), and Redis. This section covers backup strategies for each.',
        ru: 'Регулярное резервное копирование необходимо для продакшен развёртываний. PEPA хранит данные в PostgreSQL, MinIO (S3) и Redis. Этот раздел описывает стратегии резервного копирования для каждого.',
      }},
      { type: 'heading', content: { en: 'Database Backup', ru: 'Резервное копирование базы данных' }},
      { type: 'code', content: {
        en: '# One-time backup\ndocker compose exec postgres pg_dump -U pepa pepa > backup_$(date +%Y%m%d).sql\n\n# Automated daily backup (add to crontab)\n0 2 * * * cd /path/to/pepa && docker compose exec -T postgres pg_dump -U pepa pepa > /backups/pepa_$(date +\\%Y\\%m\\%d).sql\n\n# Restore from backup\ndocker compose exec -T postgres psql -U pepa pepa < backup_20250101.sql',
        ru: '# Однократное резервное копирование\ndocker compose exec postgres pg_dump -U pepa pepa > backup_$(date +%Y%m%d).sql\n\n# Автоматическое ежедневное копирование (добавьте в crontab)\n0 2 * * * cd /path/to/pepa && docker compose exec -T postgres pg_dump -U pepa pepa > /backups/pepa_$(date +\\%Y\\%m\\%d).sql\n\n# Восстановление из резервной копии\ndocker compose exec -T postgres psql -U pepa pepa < backup_20250101.sql',
      }},
      { type: 'heading', content: { en: 'MinIO / S3 Backup', ru: 'Резервное копирование MinIO / S3' }},
      { type: 'code', content: {
        en: '# Backup MinIO data volume\ndocker run --rm -v pepa_minio-data:/data -v /backups:/backup \\\n  alpine tar czf /backup/minio_$(date +%Y%m%d).tar.gz /data\n\n# Or use mc (MinIO Client) to mirror\nmc alias set pepa http://localhost:9000 minioadmin minioadmin\nmc mirror pepa/artifacts /backups/minio-artifacts/',
        ru: '# Резервное копирование volume MinIO\ndocker run --rm -v pepa_minio-data:/data -v /backups:/backup \\\n  alpine tar czf /backup/minio_$(date +%Y%m%d).tar.gz /data\n\n# Или используйте mc (MinIO Client) для зеркалирования\nmc alias set pepa http://localhost:9000 minioadmin minioadmin\nmc mirror pepa/artifacts /backups/minio-artifacts/',
      }},
      { type: 'heading', content: { en: 'Disaster Recovery', ru: 'Аварийное восстановление' }},
      { type: 'list', content: { en: '', ru: '' }, items: [
        { en: 'Keep backups in at least 2 separate locations (local + remote)', ru: 'Храните резервные копии минимум в 2 отдельных местах (локально + удалённо)' },
        { en: 'Test restore procedures regularly (monthly recommended)', ru: 'Регулярно тестируйте процедуры восстановления (рекомендуется ежемесячно)' },
        { en: 'Document recovery steps and keep them accessible', ru: 'Задокументируйте шаги восстановления и держите их доступными' },
        { en: 'Set up monitoring alerts for disk space and service health', ru: 'Настройте оповещения мониторинга для дискового пространства и здоровья сервисов' },
        { en: 'Keep a recovery VM or infrastructure-as-code for quick rebuild', ru: 'Держите recovery VM или infrastructure-as-code для быстрого пересоздания' },
      ]},
    ],
  },
];

function BlockRenderer({ block, lang }: { block: DocBlock; lang: Lang }) {
  switch (block.type) {
    case 'text':
      return <p className="mb-3 leading-relaxed">{block.content[lang]}</p>;

    case 'heading':
      return <h3 className="text-[14px] font-semibold text-[var(--text-primary)] mt-5 mb-2">{block.content[lang]}</h3>;

    case 'code':
      return (
        <pre className="bg-[var(--bg)] border border-[var(--border)] rounded-md p-3 mb-3 overflow-x-auto font-mono text-[12px] leading-relaxed text-[var(--text-primary)]">
          <code>{block.content[lang]}</code>
        </pre>
      );

    case 'list':
      return (
        <ul className="list-disc pl-5 mb-3 space-y-1">
          {block.items?.map((item, i) => (
            <li key={i} className="text-[13px] text-[var(--text-primary)]">{item[lang]}</li>
          ))}
        </ul>
      );

    case 'table':
      return (
        <div className="overflow-x-auto mb-3">
          <table className="w-full text-[12px] border border-[var(--border)] rounded-md">
            <thead>
              <tr className="bg-[var(--bg)] border-b border-[var(--border)]">
                {block.headers?.map((h, i) => (
                  <th key={i} className="text-left px-3 py-2 font-medium text-[var(--text-secondary)]">{h[lang]}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows?.map((row, i) => (
                <tr key={i} className="border-b border-[var(--border-light)] last:border-0">
                  {row[lang].map((cell, j) => (
                    <td key={j} className="px-3 py-2 text-[var(--text-primary)]">{cell}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );

    case 'image':
      return (
        <figure className="mb-4">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={block.src}
            alt={block.alt?.[lang] ?? ''}
            className="w-full rounded-md border border-[var(--border)] bg-[var(--bg)]"
            loading="lazy"
          />
          {block.alt && (
            <figcaption className="mt-1.5 text-center text-[11px] text-[var(--text-secondary)]">
              {block.alt[lang]}
            </figcaption>
          )}
        </figure>
      );

    default:
      return null;
  }
}

export default function DocumentationPage() {
  const [lang, setLang] = useState<Lang>('en');
  const [activeSection, setActiveSection] = useState<string>('getting-started');

  const currentIdx = sections.findIndex(s => s.id === activeSection);
  const currentSection = sections[currentIdx];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
    <div className="px-6 py-6 space-y-6">
      <div className="flex items-center justify-between page-animate">
        <div>
          <h2 className="page-title-modern">
            {lang === 'en' ? 'Platform Documentation' : 'Документация платформы'}
          </h2>
          <p className="page-subtitle-modern">
            {lang === 'en'
              ? 'Complete guide to working with the PEPA platform'
              : 'Полное руководство по работе с платформой PEPA'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setLang('en')}
            className={`px-3 py-1.5 text-[12px] font-medium rounded-md transition-colors ${
              lang === 'en'
                ? 'bg-[var(--accent)] text-white'
                : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--border)]'
            }`}
          >
            EN
          </button>
          <button
            onClick={() => setLang('ru')}
            className={`px-3 py-1.5 text-[12px] font-medium rounded-md transition-colors ${
              lang === 'ru'
                ? 'bg-[var(--accent)] text-white'
                : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--border)]'
            }`}
          >
            RU
          </button>
        </div>
      </div>

      <div className="flex gap-6 page-animate-up page-delay-1">
        {/* Sidebar navigation */}
        <nav className="w-[200px] shrink-0 space-y-0.5">
          {sections.map((s) => (
            <button
              key={s.id}
              onClick={() => setActiveSection(s.id)}
              className={`block w-full text-left px-3 py-1.5 rounded-md text-[12px] transition-colors ${
                activeSection === s.id
                  ? 'bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)]'
              }`}
            >
              {s.title[lang]}
            </button>
          ))}
        </nav>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <div className="card">
            <div className="card-header">
              <span className="text-[13px] font-medium text-[var(--text-primary)]">
                {currentSection.title[lang]}
              </span>
            </div>
            <div className="card-body p-5">
              {currentSection.blocks.map((block, i) => (
                <BlockRenderer key={i} block={block} lang={lang} />
              ))}
            </div>
          </div>

          {/* Navigation buttons */}
          <div className="flex justify-between mt-4">
            {currentIdx > 0 ? (
              <button
                onClick={() => setActiveSection(sections[currentIdx - 1].id)}
                className="px-4 py-2 text-[12px] text-[var(--text-secondary)] bg-[var(--surface)] border border-[var(--border)] rounded-md hover:bg-[var(--bg)] transition-colors"
              >
                &larr; {lang === 'en' ? 'Previous' : 'Назад'}
              </button>
            ) : <div />}
            {currentIdx < sections.length - 1 ? (
              <button
                onClick={() => setActiveSection(sections[currentIdx + 1].id)}
                className="px-4 py-2 text-[12px] text-white bg-[var(--accent)] rounded-md hover:opacity-90 transition-colors"
              >
                {lang === 'en' ? 'Next' : 'Далее'} &rarr;
              </button>
            ) : <div />}
          </div>
        </div>
      </div>
    </div>
    </div>
  );
}
