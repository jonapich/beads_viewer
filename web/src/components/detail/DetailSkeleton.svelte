<script lang="ts"></script>

<!-- Placeholder blocks while the first copy of an issue loads. Same grid as the page (see IssueDetail), widths as classes: the CSP allows no style attribute. -->
<div class="skel" aria-busy="true" aria-label="Loading">
  <div class="head">
    <div class="sk crumb"></div>
    <div class="sk title"></div>
    <div class="pills"><div class="sk pill-a"></div><div class="sk pill-b"></div></div>
  </div>
  <div class="rail" aria-hidden="true">
    {#each [1, 2, 3, 4, 5] as n (n)}
      <div class="field"><div class="sk lbl"></div><div class="sk val"></div></div>
    {/each}
  </div>
  <div class="main" aria-hidden="true">
    <div class="sk sec"></div>
    <div class="sk sec"></div>
    <div class="sk sec"></div>
  </div>
</div>

<style>
  .skel {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .head {
    min-width: 0;
  }
  .sk {
    background: var(--bg-hover);
    border-radius: var(--radius-sm);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .crumb {
    width: 30%;
    height: 12px;
  }
  .title {
    width: 60%;
    height: 24px;
    margin-top: var(--space-3);
  }
  .pills {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  .pill-a {
    width: 90px;
    height: 22px;
  }
  .pill-b {
    width: 48px;
    height: 22px;
  }
  .rail {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4) var(--space-5);
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }
  .lbl {
    width: 70%;
    height: 12px;
  }
  .val {
    width: 50%;
    height: 20px;
    margin-top: 6px;
  }
  .main {
    margin-top: var(--space-4);
  }
  .sec {
    width: 100%;
    height: 36px;
    margin-top: var(--space-3);
  }
  @media (max-width: 479px) {
    .rail {
      grid-template-columns: 1fr;
    }
  }
  @media (min-width: 1100px) {
    .skel {
      grid-template-columns: minmax(0, 760px) 280px;
      column-gap: var(--space-6);
    }
    .head {
      grid-column: 1 / -1;
      grid-row: 1;
    }
    .main {
      grid-column: 1;
      grid-row: 2;
    }
    .rail {
      grid-column: 2;
      grid-row: 2;
      display: flex;
      flex-direction: column;
      margin-top: var(--space-4);
      border-top: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sk {
      animation: none;
    }
  }
  @keyframes pulse {
    50% {
      opacity: 0.45;
    }
  }
</style>
