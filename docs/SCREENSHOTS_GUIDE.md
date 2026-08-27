# PEPA — Screenshot Guide

Список рекомендуемых скриншотов для документации PEPA.

---

## Приоритет 1: Основные страницы

### 1. Authentication & Setup
- [ ] **Login page** — страница входа с полями email/password
- [ ] **Initial setup** — страница первичной настройки администратора
- [ ] **Password change prompt** — запрос смены пароля при первом входе

### 2. Dashboard
- [ ] **Main dashboard** — главный дашборд со всеми виджетами
- [ ] **Dashboard customization** — режим кастомизации виджетов
- [ ] **Recent activity feed** — лента последней активности

### 3. Service Catalog
- [ ] **Service list** — список сервисов с поиском и фильтрами
- [ ] **Service detail page** — детальная страница сервиса с метаданными
- [ ] **Service creation form** — форма создания сервиса с выбором шаблона
- [ ] **Service templates** — каталог шаблонов сервисов

### 4. Deployments
- [ ] **Deployment history** — история развёртываний со статусами
- [ ] **Deployment progress** — прогресс развёртывания в реальном времени
- [ ] **Deployment logs** — логи развёртывания
- [ ] **Environment selection** — выбор окружения при деплое

---

## Приоритет 2: Инфраструктура

### 5. Kubernetes Clusters
- [ ] **Cluster list** — список кластеров с индикаторами здоровья
- [ ] **Cluster detail** — детальная информация о кластере (ноды, namespaces, ресурсы)
- [ ] **Cluster addition form** — форма добавления кластера с загрузкой kubeconfig
- [ ] **Resource usage charts** — графики использования ресурсов

### 6. Connections
- [ ] **Connections list** — список подключений с цветами статуса
- [ ] **Connection creation form** — форма создания подключения с кнопкой Test
- [ ] **Connection test result** — результат тестирования подключения
- [ ] **Different connection types** — разные типы подключений (K8s, GitLab, Jira, AI, Vault)

### 7. Docker & Virtualization
- [ ] **Docker hosts list** — список Docker хостов
- [ ] **Docker containers** — список контейнеров
- [ ] **Proxmox VMs** — список виртуальных машин
- [ ] **Proxmox containers** — список LXC контейнеров

---

## Приоритет 3: CI/CD & GitOps

### 8. Pipelines
- [ ] **Pipeline list** — список пайплайнов со статусами
- [ ] **Pipeline builder** — визуальный редактор пайплайнов
- [ ] **Pipeline blueprints** — каталог шаблонов пайплайнов
- [ ] **Pipeline run detail** — детали запуска пайплайна
- [ ] **Pipeline logs** — логи выполнения пайплайна

### 9. GitOps
- [ ] **GitOps overview** — обзор GitOps с диаграммой
- [ ] **Drift detection results** — результаты обнаружения дрейфа
- [ ] **Repository configuration** — конфигурация GitOps репозитория
- [ ] **Sync status dashboard** — дашборд статуса синхронизации

### 10. Workflows
- [ ] **Workflow list** — список рабочих процессов
- [ ] **Visual workflow designer** — визуальный DAG редактор
- [ ] **Workflow templates library** — библиотека шаблонов
- [ ] **Workflow execution** — выполнение workflow со статусами шагов
- [ ] **Workflow logs** — логи выполнения workflow

---

## Приоритет 4: Управление & Безопасность

### 11. Scorecards
- [ ] **Scorecard rules** — конфигурация правил scorecard
- [ ] **Scorecard evaluation** — результаты оценки сервиса
- [ ] **Scorecard badges** — сервисы с бейджами (Bronze/Silver/Gold/Platinum)

### 12. RBAC
- [ ] **Roles management** — управление ролями
- [ ] **Custom role creation** — создание пользовательской роли
- [ ] **Permission matrix** — матрица прав доступа с чекбоксами
- [ ] **User management** — управление пользователями

### 13. Settings
- [ ] **General settings** — общие настройки
- [ ] **AI provider configuration** — настройка AI провайдеров
- [ ] **Environment management** — управление окружениями
- [ ] **Team management** — управление командами

### 14. Vault & Credentials
- [ ] **Vault secret browser** — браузер секретов Vault
- [ ] **Credentials management** — управление учётными данными
- [ ] **Secret creation form** — форма создания секрета

---

## Приоритет 5: AI & Плагины

### 15. AI Assistant
- [ ] **AI chat interface** — интерфейс чата с AI
- [ ] **AI tools list** — список встроенных AI инструментов
- [ ] **AI provider status** — статус AI провайдеров

### 16. Plugins
- [ ] **Plugin management** — управление плагинами
- [ ] **Plugin installation** — установка плагина
- [ ] **Plugin configuration** — настройка плагина
- [ ] **Marketplace catalog** — каталог Marketplace

---

## Приоритет 6: Дополнительные страницы

### 17. Discovery & Import
- [ ] **Service discovery** — обнаружение сервисов
- [ ] **Import wizard** — мастер импорта из GitLab/GitHub

### 18. Audit & Security
- [ ] **Audit log viewer** — просмотр журнала аудита
- [ ] **Security scanning** — сканирование безопасности

### 19. Documentation
- [ ] **Built-in docs viewer** — встроенный просмотр документации

---

## Технические требования к скриншотам

### Формат
- **Формат**: PNG или WebP
- **Разрешение**: Минимум 1920x1080, рекомендуется 2560x1440
- **Качество**: Чёткое, без размытия

### Стиль
- **Тема**: Тёмная тема (если поддерживается)
- **Язык**: Английский интерфейс
- **Данные**: Реалистичные тестовые данные
- **Аннотации**: Минимальные, только если необходимо

### Именование файлов
```
screenshot-<section>-<description>.png

Примеры:
- screenshot-dashboard-main.png
- screenshot-services-list.png
- screenshot-cluster-detail.png
- screenshot-pipeline-builder.png
- screenshot-workflow-designer.png
```

### Расположение
```
docs/screenshots/
├── screenshot-dashboard-main.png
├── screenshot-services-list.png
├── screenshot-cluster-detail.png
└── ...
```

---

## Примеры использования в документации

### Markdown
```markdown
> 📸 Скриншот: Dashboard с виджетами

![Dashboard](screenshots/screenshot-dashboard-main.png)
```

### С подписью
```markdown
![Service Catalog](screenshots/screenshot-services-list.png)
*Рис. 1: Список сервисов с поиском и фильтрами*
```

---

## Чек-лист для создания скриншотов

### Подготовка
- [ ] Запустить PEPA локально (`make docker-up`)
- [ ] Создать тестовые данные (сервисы, кластеры, пайплайны)
- [ ] Настроить тёмную тему (если доступно)
- [ ] Очистить браузер (Ctrl+Shift+Delete)

### Создание
- [ ] Открыть нужную страницу
- [ ] Дождаться полной загрузки данных
- [ ] Скрыть ненужные элементы (dev tools, консоль)
- [ ] Сделать скриншот (Cmd+Shift+4 на macOS)
- [ ] Сохранить с правильным именем

### Пост-обработка
- [ ] Проверить качество изображения
- [ ] Обрезать лишние поля (если нужно)
- [ ] Добавить в `docs/screenshots/`
- [ ] Обновить документацию со ссылками

---

## Инструменты для создания скриншотов

### macOS
- **Встроенные**: Cmd+Shift+3 (весь экран), Cmd+Shift+4 (область)
- **Cleanshot**: Продвинутый инструмент с аннотациями
- **Skitch**: Бесплатный инструмент от Evernote

### Browser Extensions
- **Nimbus Screenshot**: Расширение для Chrome/Firefox
- **Fireshot**: Простой инструмент для скриншотов
- **GoFullPage**: Скриншоты всей страницы

### Online Tools
- **Lightshot**: Онлайн редактор скриншотов
- **Imgur**: Хостинг изображений
- **CloudApp**: Sharing и аннотации

---

## Следующие шаги

1. **Создать тестовые данные**:
   - 5-10 сервисов разных типов
   - 2-3 Kubernetes кластера
   - 3-5 пайплайнов
   - 2-3 workflow
   - Подключения к GitLab, Jira, Slack

2. **Начать с приоритета 1**:
   - Login, Dashboard, Services, Deployments
   - Это основные страницы, которые видят все пользователи

3. **Добавить в документацию**:
   - Обновить `user-guide-en.md` и `user-guide-ru.md`
   - Добавить ссылки на скриншоты
   - Проверить отображение

4. **Итеративно улучшать**:
   - Собратить feedback
   - Обновить скриншоты при изменении UI
   - Добавить больше деталей

---

**Создано**: 2026-08-24  
**Статус**: 📋 Готов к созданию скриншотов
