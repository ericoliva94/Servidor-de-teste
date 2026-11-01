// Alternância de abas
document.addEventListener('DOMContentLoaded', () => {
  const tabs = document.querySelectorAll('.tab[data-tab]');
  const contents = document.querySelectorAll('.tab-content');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      // Ignora o botão "Home" que é um link <a>
      if (tab.tagName === 'A') return;
      
      tabs.forEach(t => t.classList.remove('active'));
      contents.forEach(c => c.classList.remove('active'));

      tab.classList.add('active');
      const id = tab.getAttribute('data-tab');
      document.getElementById(id)?.classList.add('active');
    });
  });

  // Evento para salvar todas as configs
  const btn = document.querySelector('.btn.btn-ok'); // Botão "Salvar Configurações"
  if (btn) {
    btn.addEventListener('click', () => {
      const formData = new FormData();

      // Captura todos os inputs de todas as abas
      document.querySelectorAll('.tab-content input').forEach(input => {
        formData.append(input.name, input.value ?? '');
      });

      // Envia silenciosamente (sem alertas, conforme sua solicitação)
      fetch('/config/save', {
        method: 'POST',
        body: formData
      })
      .then(response => {
        // Opcional: você pode exibir uma mensagem discreta aqui depois
        // mas, por enquanto, segue silencioso conforme solicitado.
        console.log('Configurações enviadas.');
      })
      .catch(err => {
        console.error('Erro ao enviar configurações:', err);
      });
    });
  }
});
