import { mount } from 'svelte';
import '../app.css';
import SpectatorHandsFixture from './SpectatorHands.fixture.svelte';

// The fixture reads its URL itself (?seats, ?mode, ?preset, ?opphand).
mount(SpectatorHandsFixture, { target: document.querySelector('#fixture')! });
