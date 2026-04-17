// ==UserScript==
// @name         emby-iina
// @name:zh-CN   emby-iina
// @name:en      emby-iina
// @namespace    https://github.com/jqtmviyu/iinaServer
// @version      2026.04.17
// @description  Emby Web 调用本地 IINAServer，仅支持 IINA。
// @description:zh-CN Emby Web 调用本地 IINAServer，仅支持 IINA。
// @description:en  Forward Emby Web playback to local IINAServer for IINA only.
// @author       jqtmviyu
// @match        *://*/web/*
// @match        *://*/*/web/*
// @match        https://app.emby.media/*
// @grant        unsafeWindow
// @run-at       document-start
// @connect      127.0.0.1
// @connect      localhost
// @license      MIT
// ==/UserScript==

(function () {
    'use strict';

    const config = {
        localServer: 'http://127.0.0.1:8080',
        reviewOnly: false,
        dryRunUpload: false,
        debugMediaURL: '',
        debugSubtitleURL: '',
        dedupeWindowMs: 2500,
        requestTimeoutMs: 4000,
        debug: false,
    };

    const playButtonSelector = [
        'button.cardOverlayFab-primary[data-action="play"]',
        'button.cardOverlayFab-primary[data-action="resume"]',
        'button[data-action="play"]',
        'button[data-action="resume"]',
        'button[data-mode="play"]',
        'button[data-mode="resume"]',
    ].join(', ');
    const pageWindow = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;
    const originFetch = pageWindow.fetch.bind(pageWindow);
    const state = {
        lastPlayKey: '',
        lastPlayAt: 0,
        clickInFlight: false,
        stopInFlight: false,
        localPlaybackActive: false,
        allowNativeClickButton: null,
    };

    function log(...args) {
        if (config.debug) {
            console.log('[emby-iina]', ...args);
        }
    }

    function now() {
        return Date.now();
    }

    function trimSlash(value) {
        return value.replace(/\/+$/, '');
    }

    function toURL(input) {
        const raw = typeof input === 'string' ? input : input && typeof input.url === 'string' ? input.url : '';
        if (!raw) {
            return null;
        }
        try {
            return new URL(raw, pageWindow.location.href);
        } catch (_error) {
            return null;
        }
    }

    function buildJSONResponse(data) {
        return new Response(JSON.stringify(data), {
            status: 200,
            headers: {
                'Content-Type': 'application/json'
            },
        });
    }

    function showToast(message, tone = 'success') {
        const doc = pageWindow.document;
        if (!doc || !doc.body) {
            return;
        }
        const toast = doc.createElement('div');
        toast.textContent = message;
        toast.style.cssText = [
            'position:fixed',
            'right:24px',
            'bottom:24px',
            'z-index:2147483647',
            'max-width:420px',
            'padding:12px 16px',
            'border-radius:12px',
            tone === 'error' ? 'background:rgba(35,35,35,0.95)' : 'background:linear-gradient(135deg,#0296be 0%,#008a51 100%)',
            'color:#fff',
            'font-size:14px',
            'line-height:1.4',
            'box-shadow:0 10px 30px rgba(0,0,0,0.3)',
        ].join(';');
        doc.body.appendChild(toast);
        pageWindow.setTimeout(() => {
            toast.remove();
        }, tone === 'error' ? 3500 : 2500);
    }

    function getApiClient() {
        const apiClient = pageWindow.ApiClient;
        if (!apiClient) {
            throw new Error('ApiClient unavailable');
        }
        return apiClient;
    }

    function getApiClientInfo() {
        const apiClient = pageWindow.ApiClient;
        if (!apiClient) {
            return null;
        }
        const serverAddress = apiClient._serverAddress || apiClient._serverInfo?.Address || apiClient._serverInfo?.ManualAddress || '';
        if (!serverAddress) {
            return null;
        }
        return {
            _serverAddress: serverAddress,
            _serverVersion: apiClient._serverVersion || apiClient._serverInfo?.Version || '',
            _deviceId: apiClient._deviceId || '',
        };
    }

    function isStopped(url) {
        return Boolean(url && url.pathname.includes('/Playing/Stopped'));
    }

    function isPlayableItem(item) {
        return Boolean(item && ['Movie', 'Episode'].includes(item.Type));
    }

    function getCurrentPageItemId() {
        const hashQuery = pageWindow.location.hash.split('?')[1] || '';
        const hashId = new URLSearchParams(hashQuery).get('id');
        if (hashId) {
            return hashId;
        }
        return new URLSearchParams(pageWindow.location.search).get('id') || '';
    }

    function findPlayContext(target) {
        if (!target || typeof target.closest !== 'function') {
            return null;
        }
        const playButton = target.closest(playButtonSelector);
        if (!playButton) {
            return null;
        }
        const container = target.closest('div[is="emby-itemscontainer"]');
        const parentCard = target.closest('.virtualScrollItem.card, .backdropCard[data-index]');
        if (container && (container._itemSource || container.items) && parentCard) {
            const index = parentCard._dataItemIndex ?? parentCard.dataset.index;
            const itemList = container._itemSource || container.items;
            const item = itemList?.[index];
            if (isPlayableItem(item)) {
                return { playButton, item, itemId: item.Id };
            }
        }
        const itemId = playButton.dataset.id || playButton.dataset.itemId || target.closest('[data-id]')?.dataset.id || getCurrentPageItemId();
        if (!itemId) {
            return null;
        }
        return { playButton, item: null, itemId };
    }

    function buildPlayKey(playbackURL, playbackData) {
        const playbackInfoURL = toURL(playbackURL);
        const mediaSourceId = playbackInfoURL?.searchParams.get('MediaSourceId') || '';
        return [
            playbackURL,
            playbackData?.PlaySessionId || '',
            mediaSourceId,
        ].join('|');
    }

    function isDuplicatePlay(playKey) {
        return state.lastPlayKey === playKey && now() - state.lastPlayAt < config.dedupeWindowMs;
    }

    function rememberLocalPlay(playbackURL, playbackData) {
        state.lastPlayKey = buildPlayKey(playbackURL, playbackData);
        state.lastPlayAt = now();
        state.localPlaybackActive = true;
    }

    function allowNativeClick(playButton) {
        state.allowNativeClickButton = playButton;
        pageWindow.setTimeout(() => {
            if (state.allowNativeClickButton === playButton) {
                state.allowNativeClickButton = null;
            }
        }, 1500);
    }

    function shouldPassThroughClick(target) {
        if (!target) {
            return false;
        }
        const playButton = state.allowNativeClickButton;
        if (!playButton) {
            return false;
        }
        const matched = target === playButton || playButton.contains(target);
        if (matched) {
            state.allowNativeClickButton = null;
        }
        return matched;
    }

    function buildPlaybackURL(itemId, playbackData, mainEpInfo) {
        const apiClient = getApiClient();
        const userId = apiClient?._serverInfo?.UserId || '';
        const deviceId = apiClient?._deviceId || '';
        const accessToken = apiClient?._userAuthInfo?.AccessToken || apiClient?._serverInfo?.AccessToken || '';
        const mediaSourceId = playbackData?.MediaSources?.[0]?.Id || '';
        const startTimeTicks = mainEpInfo?.UserData?.PlaybackPositionTicks || 0;
        const searchParams = new URLSearchParams({
            'X-Emby-Device-Id': deviceId,
            'StartTimeTicks': String(startTimeTicks),
            'X-Emby-Token': accessToken,
            'UserId': userId,
            'IsPlayback': 'true',
        });
        if (mediaSourceId) {
            searchParams.set('MediaSourceId', mediaSourceId);
        }
        const apiClientInfo = getApiClientInfo();
        const serverAddress = apiClientInfo?._serverAddress || pageWindow.location.origin;
        return `${trimSlash(serverAddress)}/emby/Items/${itemId}/PlaybackInfo?${searchParams.toString()}`;
    }

    async function getItemPlaybackInfo(itemId) {
        const apiClient = getApiClient();
        if (typeof apiClient.getPlaybackInfo !== 'function') {
            throw new Error('ApiClient.getPlaybackInfo unavailable');
        }
        return apiClient.getPlaybackInfo(itemId);
    }

    async function getMainEpInfo(itemId) {
        const apiClient = getApiClient();
        const userId = apiClient?._serverInfo?.UserId;
        if (typeof apiClient.getItem !== 'function' || !userId) {
            throw new Error('ApiClient.getItem unavailable');
        }
        return apiClient.getItem(userId, itemId);
    }

    async function buildPlayData(itemId) {
        const [playbackData, mainEpInfo] = await Promise.all([
            getItemPlaybackInfo(itemId),
            getMainEpInfo(itemId),
        ]);
        if (!playbackData || !Array.isArray(playbackData.MediaSources) || playbackData.MediaSources.length === 0) {
            throw new Error('missing playback media sources');
        }
        return {
            playbackURL: buildPlaybackURL(itemId, playbackData, mainEpInfo),
            playbackData,
            mainEpInfo,
            requestHeaders: {
                Referer: pageWindow.location.href,
            },
        };
    }

    async function postJSON(path, data) {
        const controller = new AbortController();
        const timeoutId = pageWindow.setTimeout(() => {
            controller.abort();
        }, config.requestTimeoutMs);
        try {
            const response = await originFetch(trimSlash(config.localServer) + path, {
                method: 'POST',
                mode: 'cors',
                signal: controller.signal,
                headers: {
                    'Content-Type': 'text/plain'
                },
                body: JSON.stringify(data),
            });
            return {
                status: response.status,
                responseText: await response.text(),
            };
        } catch (error) {
            if (error?.name === 'AbortError') {
                throw new Error(`request timeout after ${config.requestTimeoutMs}ms`);
            }
            throw error;
        } finally {
            pageWindow.clearTimeout(timeoutId);
        }
    }

    function buildPayload(playData) {
        const options = {
            reviewOnly: config.reviewOnly,
            dryRunUpload: config.dryRunUpload,
        };
        if (config.debugMediaURL) {
            options.debugMediaURL = config.debugMediaURL;
        }
        if (config.debugSubtitleURL) {
            options.debugSubtitleURL = config.debugSubtitleURL;
        }
        return {
            ApiClient: getApiClientInfo(),
            playbackUrl: playData.playbackURL,
            playbackData: {
                PlaySessionId: playData.playbackData?.PlaySessionId || '',
                MediaSources: Array.isArray(playData.playbackData?.MediaSources) ? playData.playbackData.MediaSources : [],
            },
            request: {
                headers: playData.requestHeaders,
            },
            extraData: {
                mainEpInfo: playData.mainEpInfo || {},
            },
            mountDiskEnable: 'false',
            options,
        };
    }

    async function handleLocalPlay(playData) {
        const payload = buildPayload(playData);
        log('forward playback payload', payload);
        const response = await postJSON('/v1/emby/play', payload);
        if (response.status >= 200 && response.status < 300) {
            rememberLocalPlay(playData.playbackURL, playData.playbackData);
            log('forward playback success', state.lastPlayKey, response.responseText);
            return;
        }
        throw new Error(`local play status=${response.status} body=${response.responseText || ''}`);
    }

    async function handleLocalStop() {
        if (state.stopInFlight) {
            return true;
        }
        state.stopInFlight = true;
        try {
            const response = await postJSON('/v1/session/stop', {});
            if ((response.status >= 200 && response.status < 300) || response.status === 404) {
                log('forward stop success', response.status);
                return true;
            }
            throw new Error(`local stop status=${response.status}`);
        } catch (error) {
            log('forward stop failed', error);
            return false;
        } finally {
            state.localPlaybackActive = false;
            state.stopInFlight = false;
        }
    }

    function fallbackToNativePlay(context, error) {
        log('click play failed', error);
        showToast(`点击接管失败：${error?.message || error}`, 'error');
        allowNativeClick(context.playButton);
        pageWindow.setTimeout(() => {
            context.playButton.click();
        }, 0);
    }

    async function handleClickPlay(context) {
        if (state.clickInFlight) {
            return;
        }
        state.clickInFlight = true;
        try {
            const itemId = context.item?.Id || context.itemId;
            if (!itemId) {
                throw new Error('item id missing');
            }
            const playData = await buildPlayData(itemId);
            const playKey = buildPlayKey(playData.playbackURL, playData.playbackData);
            if (isDuplicatePlay(playKey) && state.localPlaybackActive) {
                log('duplicate click play ignored', playKey);
                return;
            }
            await handleLocalPlay(playData);
            showToast('已调用 IINA · 已转交本地播放器处理');
        } catch (error) {
            fallbackToNativePlay(context, error);
        } finally {
            state.clickInFlight = false;
        }
    }

    pageWindow.document.addEventListener('click', function (event) {
        if (shouldPassThroughClick(event.target)) {
            return;
        }
        const context = findPlayContext(event.target);
        if (!context || state.clickInFlight) {
            return;
        }
        log('take over play click', context.itemId || context.item?.Id || '', context.playButton?.outerHTML || '');
        event.preventDefault();
        event.stopImmediatePropagation();
        void handleClickPlay(context);
    }, true);

    pageWindow.fetch = async function (input, init) {
        const url = toURL(input);
        if (!isStopped(url) || !state.localPlaybackActive) {
            return originFetch(input, init);
        }
        const stopped = await handleLocalStop();
        if (stopped) {
            return buildJSONResponse({});
        }
        return originFetch(input, init);
    };
})();
