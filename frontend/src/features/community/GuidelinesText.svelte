<script lang="ts">
  import { guidelines } from "../../lib/community";
</script>

<div class="guidelines">
  <p class="guidelines-lead">
    Die papierleicht-Community ist ein geschützter Raum für Unternehmerinnen und Unternehmer. Damit
    das so bleibt, gelten diese Regeln im Chat und unter Angebote &amp; Gesuche.
  </p>
  {#each guidelines as section, index}
    <section>
      <h3>{index + 1}. {section.title}</h3>
      <ul>
        {#each section.items as item}<li>{item}</li>{/each}
      </ul>
    </section>
  {/each}
</div>

<style>
  .guidelines {
    display: grid;
    gap: 12px;
    font-size: 13px;
    line-height: 1.55;
  }
  .guidelines-lead {
    margin: 0;
    color: var(--theme-muted);
  }
  h3 {
    margin: 0 0 4px;
    font-size: 13px;
  }
  ul {
    margin: 0;
    padding-left: 18px;
  }
  li + li {
    margin-top: 3px;
  }
</style>
