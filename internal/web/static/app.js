const $ = selector => document.querySelector(selector);
const state = { job: null, source: null, file: null };

function showNote(message = '', kind = '') {
  const element = $('#note');
  element.textContent = message;
  element.dataset.kind = kind;
}

function formatNumber(value) {
  return new Intl.NumberFormat('ru-RU').format(value || 0);
}

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / 1024 / 1024).toFixed(1)} МБ`;
}

function setFile(file) {
  state.file = file || null;
  $('#fileInfo').classList.toggle('hidden', !state.file);
  if (!state.file) {
    $('#file').value = '';
    return;
  }
  $('#fileName').textContent = state.file.name;
  $('#fileSize').textContent = formatBytes(state.file.size);
  showNote('Файл готов к загрузке.', 'success');
}

fetch('/api/config')
  .then(response => response.json())
  .then(config => $('#credentialWrap').classList.toggle('hidden', config.credentialConfigured))
  .catch(() => showNote('Не удалось загрузить настройки приложения.', 'error'));

const drop = $('#drop');
drop.addEventListener('click', () => $('#file').click());
drop.addEventListener('keydown', event => {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    $('#file').click();
  }
});
drop.addEventListener('dragover', event => {
  event.preventDefault();
  drop.classList.add('over');
});
drop.addEventListener('dragleave', () => drop.classList.remove('over'));
drop.addEventListener('drop', event => {
  event.preventDefault();
  drop.classList.remove('over');
  const file = event.dataTransfer.files[0];
  if (file) setFile(file);
});
$('#file').addEventListener('change', event => setFile(event.target.files[0]));
$('#clearFile').addEventListener('click', event => {
  event.stopPropagation();
  setFile(null);
  showNote('Файл убран.');
});

$('#start').addEventListener('click', async () => {
  const text = $('#codes').value;
  const file = state.file || $('#file').files[0];
  if (!file && !text.trim()) {
    showNote('Добавьте коды в поле или выберите файл.', 'error');
    $('#codes').focus();
    return;
  }

  const button = $('#start');
  button.disabled = true;
  button.querySelector('span').textContent = 'Создаём задачу…';
  showNote('Подготавливаем данные…');

  try {
    const form = new FormData();
    form.append('text', text);
    form.append('credential', $('#credential')?.value || '');
    if (file) form.append('file', file);

    const response = await fetch('/api/jobs', { method: 'POST', body: form });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error || 'Не удалось создать задачу');

    state.job = payload.id;
    $('#progress').classList.remove('hidden');
    $('#results').classList.add('hidden');
    $('#cancel').classList.remove('hidden');
    showNote('Проверка запущена.', 'success');
    renderProgress(payload);
    watch(payload.id);
    $('#progress').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  } catch (error) {
    showNote(error.message, 'error');
  } finally {
    button.disabled = false;
    button.querySelector('span').textContent = 'Начать проверку';
  }
});

function watch(id) {
  if (state.source) state.source.close();
  state.source = new EventSource(`/api/jobs/${id}/events`);
  state.source.onmessage = event => {
    const snapshot = JSON.parse(event.data);
    renderProgress(snapshot);
    if (['completed', 'completed_with_errors', 'cancelled', 'failed'].includes(snapshot.state)) {
      state.source.close();
      $('#cancel').classList.add('hidden');
      if (snapshot.state.startsWith('completed')) loadResults(id, snapshot);
      if (snapshot.state === 'cancelled') showNote('Проверка отменена. Уже полученные результаты сохранены.');
      if (snapshot.state === 'failed') showNote(snapshot.message || 'Проверка завершилась с ошибкой.', 'error');
    }
  };
  state.source.onerror = () => {
    if (state.source.readyState === EventSource.CLOSED) return;
    showNote('Соединение с задачей прервано. Браузер попробует подключиться снова.', 'error');
  };
}

const stateLabels = {
  parsing: 'Разбор данных',
  queued: 'В очереди',
  running: 'Выполняется',
  completed: 'Завершено',
  completed_with_errors: 'Есть ошибки',
  cancelled: 'Отменено',
  failed: 'Ошибка'
};

function renderProgress(snapshot) {
  const percent = snapshot.uniqueCodes ? Math.round(snapshot.checked / snapshot.uniqueCodes * 100) : 0;
  $('.bar i').style.width = `${percent}%`;
  $('#progressPercent').textContent = `${percent}%`;
  $('#jobState').textContent = stateLabels[snapshot.state] || snapshot.state;
  $('#statTotal').textContent = formatNumber(snapshot.totalLines);
  $('#statUnique').textContent = formatNumber(snapshot.uniqueCodes);
  $('#statChecked').textContent = formatNumber(snapshot.checked);
  $('#statValid').textContent = formatNumber(snapshot.valid);
  $('#statInvalid').textContent = formatNumber(snapshot.invalid);
  $('#statErrors').textContent = formatNumber(snapshot.errors);
}

async function loadResults(id, snapshot) {
  try {
    const response = await fetch(`/api/jobs/${id}/invalid`);
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error || 'Не удалось получить результаты');

    const rows = payload.invalidResults || [];
    const body = $('tbody');
    body.replaceChildren(...rows.map(item => {
      const row = document.createElement('tr');
      [item.code, item.tags, item.lines.join(' '), item.reason].forEach(value => {
        const cell = document.createElement('td');
        cell.textContent = value;
        row.append(cell);
      });
      return row;
    }));

    $('#invalidCSV').href = `/api/jobs/${id}/invalid.csv`;
    $('#errorCSV').href = `/api/jobs/${id}/errors.csv`;
    $('#errorCSV').classList.toggle('hidden', !snapshot.errors);
    $('#emptyResult').classList.toggle('hidden', rows.length !== 0);
    $('#resultTable').classList.toggle('hidden', rows.length === 0);
    $('#results').classList.remove('hidden');
    showNote(
      snapshot.state === 'completed' ? 'Проверка успешно завершена.' : 'Проверка завершена с техническими ошибками.',
      snapshot.state === 'completed' ? 'success' : 'error'
    );
    $('#results').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  } catch (error) {
    showNote(error.message, 'error');
  }
}

$('#cancel').addEventListener('click', async () => {
  if (!state.job) return;
  $('#cancel').disabled = true;
  try {
    const response = await fetch(`/api/jobs/${state.job}`, { method: 'DELETE' });
    if (!response.ok) throw new Error('Не удалось отменить проверку');
    showNote('Отмена запрошена…');
  } catch (error) {
    $('#cancel').disabled = false;
    showNote(error.message, 'error');
  }
});
