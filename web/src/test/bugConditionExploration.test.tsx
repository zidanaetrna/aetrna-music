/**
 * Bug Condition Exploration Tests — Task 1
 *
 * These tests encode the EXPECTED (correct) behavior.
 * They are run against the UNFIXED code and are expected to FAIL.
 * Failure confirms each bug exists and is observable.
 *
 * DO NOT attempt to fix the tests or the implementation when they fail.
 *
 * Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.10, 1.11
 */

import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';

// ─── Shared test wrapper ────────────────────────────────────────────────────

// Minimal I18n context so components don't throw
const I18nContext = React.createContext<any>(undefined);
const ToastContext = React.createContext<any>(undefined);

const mockT = (key: string, params: Record<string, string> = {}): string => {
    // Return the key itself as a stand-in translation
    let val = key;
    Object.keys(params).forEach(k => { val = val.replace(`{${k}}`, params[k]); });
    return val;
};

const TestWrapper: React.FC<{ children: React.ReactNode }> = ({ children }) => (
    <I18nContext.Provider value={{ t: mockT, language: 'en', setLanguage: () => {} }}>
        <ToastContext.Provider value={{ showToast: () => {}, toasts: [], removeToast: () => {} }}>
            {children}
        </ToastContext.Provider>
    </I18nContext.Provider>
);

// Patch the module-level context hooks so component imports use our mocks
vi.mock('../context/I18nContext', () => ({
    useI18n: () => ({ t: mockT, language: 'en', setLanguage: () => {} }),
}));

vi.mock('../context/ToastContext', () => ({
    useToast: () => ({ showToast: vi.fn(), toasts: [], removeToast: vi.fn() }),
}));

// ─── WebSocket mock (useWebSocket returns empty telemetry) ───────────────────
vi.mock('../hooks/useWebSocket', () => ({
    useWebSocket: () => ({ connected: false, telemetry: {} }),
}));

// ─── Imports (after mocks are registered) ───────────────────────────────────
import { OverviewTab } from '../components/OverviewTab';
import { TopBar } from '../components/TopBar';
import { LoginModal } from '../components/LoginModal';

// ────────────────────────────────────────────────────────────────────────────
// Test 1 — OverviewTab: hardcoded track title and uptime
// Expected outcome: FAILS on unfixed code because the component ignores the
// mocked API and renders "Aetrna — Echoes of Eternity" and "48h 12m" instead.
// ────────────────────────────────────────────────────────────────────────────
describe('Bug Condition Exploration — Test 1: OverviewTab hardcoded track & uptime', () => {
    const MOCK_STATUS = {
        guildCount: 3,
        ramMB: 88,
        uptime: '2h 5m',
        hasCookies: true,
        nowPlaying: {
            title: 'Live Track',
            author: 'Live Artist',
            duration: '3:30',
            thumbnail: 'https://example.com/thumb.png',
            requested: '@testuser',
        },
        queue: [],
        clientIP: '127.0.0.1',
    };

    it('should render the live track title "Live Track" from /api/status (not the hardcoded "Aetrna — Echoes of Eternity")', async () => {
        render(
            <TestWrapper>
                <OverviewTab selectedGuild="guild-123" token="test-token" status={MOCK_STATUS} />
            </TestWrapper>
        );

        // Wait for any async data fetch to settle
        await waitFor(() => {}, { timeout: 500 });

        // EXPECTED (correct) behavior: live title from mock
        const h3 = screen.queryByText(/Live Track/i);
        expect(h3).not.toBeNull();

        // Counterexample if this assertion fails:
        // "Hardcoded 'Aetrna — Echoes of Eternity' appears in <h3> despite mock returning 'Live Track'"
        const hardcodedTitle = screen.queryByText(/Echoes of Eternity/i);
        expect(hardcodedTitle).toBeNull();
    });

    it('should render uptime "2h 5m" from /api/status (not the hardcoded "48h 12m")', async () => {
        render(
            <TestWrapper>
                <OverviewTab selectedGuild="guild-123" token="test-token" status={MOCK_STATUS} />
            </TestWrapper>
        );

        await waitFor(() => {}, { timeout: 500 });

        // EXPECTED: live uptime string
        expect(screen.queryByText('2h 5m')).not.toBeNull();

        // Counterexample if this assertion fails:
        // "Hardcoded '48h 12m' appears in uptime stat card despite mock returning '2h 5m'"
        expect(screen.queryByText('48h 12m')).toBeNull();
    });
});

// ────────────────────────────────────────────────────────────────────────────
// Test 2 — TopBar: hardcoded guild dropdown options
// Expected outcome: FAILS on unfixed code because TopBar never calls
// /api/guilds and instead renders 4 hardcoded <option> elements.
// ────────────────────────────────────────────────────────────────────────────
describe('Bug Condition Exploration — Test 2: TopBar hardcoded guild options', () => {
    const MOCK_GUILDS = [
        { id: 'guild-aaa', name: 'Guild Alpha', memberCount: 10, status: 'playing' },
        { id: 'guild-bbb', name: 'Guild Beta',  memberCount: 5,  status: 'idle' },
    ];

    let fetchSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
            ok: true,
            json: async () => MOCK_GUILDS,
        } as Response);
    });

    afterEach(() => {
        fetchSpy.mockRestore();
    });

    it('should render exactly 2 <option> elements matching the mocked guild IDs (not 4 hardcoded options)', async () => {
        const setSelectedGuild = vi.fn();

        render(
            <TestWrapper>
                <TopBar
                    activeTab="overview"
                    selectedGuild="guild-aaa"
                    setSelectedGuild={setSelectedGuild}
                    token="test-token"
                />
            </TestWrapper>
        );

        await waitFor(() => {}, { timeout: 500 });

        const select = document.querySelector('select#guildSelect') as HTMLSelectElement | null;
        expect(select).not.toBeNull();

        const options = select!.querySelectorAll('option');

        // EXPECTED: exactly 2 options from mock
        // Counterexample if this assertion fails:
        // "4 hardcoded guild options appear despite mock /api/guilds returning 2 entries"
        expect(options.length).toBe(2);

        const optionValues = Array.from(options).map(o => o.value);
        expect(optionValues).toContain('guild-aaa');
        expect(optionValues).toContain('guild-bbb');

        // None of the hardcoded IDs should appear
        expect(optionValues).not.toContain('102938475610293847');
        expect(optionValues).not.toContain('293847102938471029');
        expect(optionValues).not.toContain('384729103847291038');
        expect(optionValues).not.toContain('482910384729103847');
    });
});

// ────────────────────────────────────────────────────────────────────────────
// Test 3 — LoginModal: auth bypass on network error
// Expected outcome: FAILS on unfixed code because the catch block calls
// onLoginSuccess('local_dev_token') instead of showing an error message.
// ────────────────────────────────────────────────────────────────────────────
describe('Bug Condition Exploration — Test 3: LoginModal auth bypass on network error', () => {
    let fetchSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        // Simulate a network-level throw (no response at all)
        fetchSpy = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Network request failed'));
    });

    afterEach(() => {
        fetchSpy.mockRestore();
    });

    it('should NOT call onLoginSuccess when fetch throws a network error', async () => {
        const onLoginSuccess = vi.fn();

        const { container } = render(
            <TestWrapper>
                <LoginModal onLoginSuccess={onLoginSuccess} />
            </TestWrapper>
        );

        // Fill in the password field and submit the form directly
        const passwordInput = screen.getByPlaceholderText(/Enter your password/i);
        fireEvent.change(passwordInput, { target: { value: 'somepassword' } });

        const form = container.querySelector('form.split-auth-form') as HTMLFormElement;
        fireEvent.submit(form);

        // Wait for the async handler to run
        await waitFor(() => {}, { timeout: 500 });

        // EXPECTED: onLoginSuccess is never called
        // Counterexample if this assertion fails:
        // "onLoginSuccess('local_dev_token') was called despite a network error — auth bypass confirmed"
        expect(onLoginSuccess).not.toHaveBeenCalled();
    });

    it('should display an error message when fetch throws a network error', async () => {
        const onLoginSuccess = vi.fn();

        const { container } = render(
            <TestWrapper>
                <LoginModal onLoginSuccess={onLoginSuccess} />
            </TestWrapper>
        );

        const passwordInput = screen.getByPlaceholderText(/Enter your password/i);
        fireEvent.change(passwordInput, { target: { value: 'somepassword' } });

        const form = container.querySelector('form.split-auth-form') as HTMLFormElement;
        fireEvent.submit(form);

        await waitFor(() => {}, { timeout: 500 });

        // EXPECTED: an error message is shown to the user
        // Counterexample if this assertion fails:
        // "No error message rendered — component silently granted access via local_dev_token"
        const errorEl = document.querySelector('.auth-error-alert');
        expect(errorEl).not.toBeNull();
        expect(errorEl!.textContent).toMatch(/network error|unable to reach|try again/i);
    });
});

// ────────────────────────────────────────────────────────────────────────────
// Test 4 — OverviewTab.handleAction: missing Authorization header
// Expected outcome: FAILS on unfixed code because handleAction calls fetch
// without an Authorization header.
// ────────────────────────────────────────────────────────────────────────────
describe('Bug Condition Exploration — Test 4: OverviewTab handleAction Authorization header', () => {
    let capturedRequest: RequestInit | undefined;
    let fetchSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        capturedRequest = undefined;
        fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation((_url: RequestInfo | URL, init?: RequestInit) => {
            capturedRequest = init;
            return Promise.resolve({
                ok: true,
                json: async () => ({}),
            } as Response);
        });
    });

    afterEach(() => {
        fetchSpy.mockRestore();
    });

    it('should include Authorization: Bearer test-token in the /api/control fetch call', async () => {
        render(
            <TestWrapper>
                <OverviewTab selectedGuild="guild-123" token="test-token" status={null} />
            </TestWrapper>
        );

        // Click the Pause button to trigger handleAction('pause', ...)
        const pauseButton = screen.getByRole('button', { name: /pause/i });
        fireEvent.click(pauseButton);

        await waitFor(() => {
            // fetch must have been called at some point
            expect(fetchSpy).toHaveBeenCalled();
        }, { timeout: 1000 });

        // EXPECTED: the intercepted /api/control request carries the auth header
        // Counterexample if this assertion fails:
        // "fetch('/api/control') has no Authorization header — unauthenticated control request confirmed"
        const headers = capturedRequest?.headers as Record<string, string> | undefined;
        expect(headers).toBeDefined();
        expect(headers!['Authorization']).toBe('Bearer test-token');
    });
});
