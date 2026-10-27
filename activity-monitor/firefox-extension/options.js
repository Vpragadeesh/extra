const paused = document.getElementById('paused');
const excluded = document.getElementById('excluded');
const status = document.getElementById('status');
const saveBtn = document.getElementById('save');
const cancelBtn = document.getElementById('cancel');

async function loadSettings() {
  const v = await browser.storage.local.get({ paused: false, excludedDomains: [] });
  paused.checked = v.paused;
  excluded.value = v.excludedDomains.join('\n');
  updateStatus('Settings loaded.', 'saved');
}

async function saveSettings() {
  const excludedDomains = excluded.value.split('\n')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
  
  await browser.storage.local.set({ paused: paused.checked, excludedDomains });
  await browser.storage.local.get({ paused: false, excludedDomains: [] }).then((v) => {
    // Notify the background script about the pause state change
    try {
      browser.runtime.sendMessage({ type: 'setPaused', paused: v.paused });
    } catch (e) {
      // Message port may not be ready yet; ignore
    }
  });
  updateStatus('Settings saved.', 'saved');
}

function updateStatus(msg, type = '') {
  status.textContent = msg;
  status.className = type === 'error' ? 'error' : 'saved';
}

// Save on button click
saveBtn.addEventListener('click', async () => {
  try {
    await saveSettings();
  } catch (e) {
    updateStatus('Error saving settings.', 'error');
    console.error(e);
  }
});

// Cancel button
cancelBtn.addEventListener('click', () => {
  loadSettings();
  updateStatus('Cancelled.', '');
});

// Initialize
loadSettings();