<script lang="ts">
  import TableEventOverlay from './TableEventOverlay.svelte';

  // The table page's binding, exposed for the test: Table.svelte keeps the
  // board (not the idle match list) while this reads true.
  let inPlay = $state(false);
</script>

<output data-testid="in-play">{inPlay}</output>
<TableEventOverlay table="e1-r1-s03-g1-a1" onInPlay={(v) => (inPlay = v)} />
