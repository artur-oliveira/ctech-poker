#!/usr/bin/env python3
"""Build an offline, searchable gallery from Flutter's exported screenshots."""
import html
import json
from pathlib import Path

root = Path(__file__).resolve().parent.parent / 'docs' / 'screenshots'
items = json.loads((root / 'manifest.json').read_text())
cards = '\n'.join(
    f'<figure data-title="{html.escape(item["title"], quote=True)}"><a href="{item["file"]}" target="_blank" rel="noopener"><img src="{item["file"]}" loading="lazy" alt="{html.escape(item["title"], quote=True)}"></a><figcaption>{i:02d} · {html.escape(item["title"])}</figcaption></figure>'
    for i, item in enumerate(items, 1)
)
(root / 'index.html').write_text('''<!doctype html>
<html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CTech Poker · Revisão mobile</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#120d0e;color:#f6f0e7;font:16px/1.5 system-ui,sans-serif}header{padding:32px;max-width:1000px}h1{font-size:32px;margin:0 0 12px}p{color:#cbbfc0}input{width:100%;max-width:480px;padding:14px;background:#211416;color:#f6f0e7;border:1px solid #ad9fa0;border-radius:8px;font:inherit}input:focus-visible,a:focus-visible{outline:3px solid #ed777c;outline-offset:4px}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:32px;padding:0 32px 48px;align-items:start}figure{margin:0}img{display:block;width:100%;height:auto;border:1px solid #3e3133}figcaption{padding:12px 0;font-weight:600}figure[hidden]{display:none}a{color:inherit}
</style>
<header><h1>CTech Poker · Revisão mobile</h1><p>''' + str(len(items)) + ''' prints dos widgets Flutter com dados demonstrativos. Toque em uma imagem para abrir em tamanho completo. Para comentar, use o número e o nome da tela.</p><p>Capturas de teste, sem barras do sistema ou teclado. Login externo, permissões, microfone e desafio Turnstile ativo dependem de aparelho e integração. A mesa usa snapshot fixo; não representa uma partida conectada.</p><label for="search">Buscar tela</label><br><input id="search" type="search" placeholder="Ex.: mesa, perfil, loja" autocomplete="off"><p id="count" role="status"></p></header><main>''' + cards + '''</main>
<script>
const search=document.querySelector('#search');const figures=[...document.querySelectorAll('figure')];const normalize=s=>s.normalize('NFD').replace(/[\\u0300-\\u036f]/g,'').toLowerCase();search.addEventListener('input',()=>{let count=0;for(const f of figures){f.hidden=!normalize(f.dataset.title).includes(normalize(search.value));if(!f.hidden)count++}document.querySelector('#count').textContent=count+' imagens'});
</script></html>''')
print(f'{len(items)} screenshots: {root / "index.html"}')

# Keep the user's original six references available beside the revised screens.
references = [('01', 'Entrar'), ('02', 'Lobby'), ('04', 'Mãos · Histórico'),
              ('05', 'Mãos · Estatísticas'), ('06', 'Mãos · Conquistas'),
              ('38', 'Mesa · nove jogadores')]
comparisons = []
for original, title in references:
    current = next((item for item in items if item['title'] == title), None)
    if current and (root.parent / 'screenshots-before' / f'{original}.png').exists():
        comparisons.append(f'<section><h2>{original}.png · {html.escape(title)}</h2><div class="pair"><figure><figcaption>Antes</figcaption><img loading="lazy" src="../screenshots-before/{original}.png" alt="Antes: {html.escape(title)}"></figure><figure><figcaption>Agora</figcaption><a href="{current["file"]}"><img loading="lazy" src="{current["file"]}" alt="Agora: {html.escape(title)}"></a></figure></div></section>')
(root / 'comparison.html').write_text('''<!doctype html><html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>CTech Poker · Antes e depois</title><style>
*{box-sizing:border-box}body{background:#120d0e;color:#f6f0e7;font:16px/1.5 system-ui,sans-serif;max-width:1100px;margin:0 auto;padding:24px}h1{font-size:32px}h2{font-size:20px;margin-top:48px}a{color:#ed777c}.pair{display:grid;grid-template-columns:1fr 1fr;gap:24px;align-items:start}figure{margin:0}figcaption{padding:12px 0;color:#cbbfc0}img{display:block;width:100%;height:auto;border:1px solid #3e3133}@media(max-width:500px){body{padding:12px}.pair{gap:8px}}
</style><h1>Revisão dos seus seis prints</h1><p>Mesmas telas, com ícones e cartas do web, nomes corrigidos e nova composição. A numeração abaixo corresponde aos prints originais do feedback.</p><p><a href="index.html">Abrir a galeria completa atualizada</a></p>''' + '\n'.join(comparisons) + '</html>')
