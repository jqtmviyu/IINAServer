// ==UserScript==
// @name         emby-iina-only
// @name:zh-CN   emby-iina-only
// @name:en      emby-iina-only
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
// @grant        GM_xmlhttpRequest
// @run-at       document-start
// @connect      127.0.0.1
// @connect      localhost
// @license      MIT
// ==/UserScript==

'use strict';
/*global ApiClient, GM_xmlhttpRequest */

(function () {
    'use strict';

    const config = {
        localServer: 'http://127.0.0.1:8080',
        reviewOnly: false,
        dryRunUpload: true,
        debugMediaURL: '',
        debugSubtitleURL: '',
        dedupeWindowMs: 2500,
        requestTimeoutMs: 4000,
        debug: false,
    };

    const pageWindow = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;
    const originFetch = pageWindow.fetch.bind(pageWindow);
    const state = {
        lastPlayKey: '',
        lastPlayAt: 0,
        localPlaybackActive: false,
        stopInFlight: false,
        lastNoticeAt: 0,
        passThroughClickOnce: false,
        clickPlayInFlight: false,
        suppressPlaybackErrorUntil: 0,
        suppressPlaybackRouteUntil: 0,
        lastNonPlaybackURL: pageWindow.location.href,
    };

    function log(...args) {
        if (config.debug) {
            console.log('[emby-iina-only]', ...args);
        }
    }

    function now() {
        return Date.now();
    }

    function trimSlash(value) {
        return value.replace(/\/+$/, '');
    }

    function getURLString(input) {
        if (typeof input === 'string') {
            return input;
        }
        if (input && typeof input.url === 'string') {
            return input.url;
        }
        return '';
    }

    function parseURL(input) {
        const raw = getURLString(input);
        if (!raw) {
            return null;
        }
        try {
            return new URL(raw, pageWindow.location.href);
        } catch (_error) {
            return null;
        }
    }

    function headersToObject(headersLike) {
        if (!headersLike) {
            return {};
        }
        if (headersLike instanceof Headers) {
            return Object.fromEntries(headersLike.entries());
        }
        if (Array.isArray(headersLike)) {
            return Object.fromEntries(headersLike);
        }
        if (typeof headersLike.forEach === 'function') {
            const result = {};
            headersLike.forEach((value, key) => {
                result[key] = value;
            });
            return result;
        }
        if (typeof headersLike === 'object') {
            return { ...headersLike };
        }
        return {};
    }

    function getRequestHeaders(input, init) {
        const fromInit = headersToObject(init && init.headers);
        const fromInput = headersToObject(input && typeof input !== 'string' ? input.headers : null);
        const headers = { ...fromInput, ...fromInit };
        if (!headers.Referer) {
            headers.Referer = pageWindow.location.href;
        }
        return headers;
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

    function isPlaybackInfo(url) {
        return Boolean(url && url.pathname.includes('/Items/') && url.pathname.includes('/PlaybackInfo') && url.searchParams.get('IsPlayback') === 'true');
    }

    function isStopped(url) {
        return Boolean(url && url.pathname.includes('/Playing/Stopped'));
    }

    function isPlaybackRouteURL(urlString) {
        return typeof urlString === 'string' && urlString.includes('/videoosd/videoosd');
    }

    function updateLastNonPlaybackURL(urlString) {
        if (!isPlaybackRouteURL(urlString)) {
            state.lastNonPlaybackURL = urlString;
        }
    }

    function shouldSuppressPlaybackRoute(urlString) {
        return now() < state.suppressPlaybackRouteUntil && isPlaybackRouteURL(urlString);
    }

    function buildJSONResponse(data) {
        return new Response(JSON.stringify(data), {
            status: 200,
            headers: {
                'Content-Type': 'application/json'
            },
        });
    }

    function isPlayableItem(item) {
        return Boolean(item && ['Movie', 'Episode'].includes(item.Type));
    }

    function markPassThroughClick() {
        state.passThroughClickOnce = true;
        pageWindow.setTimeout(() => {
            state.passThroughClickOnce = false;
        }, 1500);
    }

    function shouldSuppressPlaybackError(reason) {
        if (now() > state.suppressPlaybackErrorUntil) {
            return false;
        }
        const title = reason?.errorTitle || reason?.message || reason?.msg || '';
        return typeof title === 'string' && title.includes('播放错误');
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
        const playButton = target.closest('button.cardOverlayFab-primary[data-action="play"], button.cardOverlayFab-primary[data-action="resume"], button[data-action="play"], button[data-action="resume"], button[data-mode="play"], button[data-mode="resume"]');
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
        let mediaSourceId = '';
        try {
            mediaSourceId = new URL(playbackURL, pageWindow.location.href).searchParams.get('MediaSourceId') || '';
        } catch (_error) {
            mediaSourceId = '';
        }
        return [
            playbackURL,
            playbackData?.PlaySessionId || '',
            mediaSourceId,
        ].join('|');
    }

    function buildPlaybackURL(itemId, playbackData, mainEpInfo) {
        const apiClient = pageWindow.ApiClient;
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
        const apiClient = pageWindow.ApiClient;
        if (!apiClient || typeof apiClient.getPlaybackInfo !== 'function') {
            throw new Error('ApiClient.getPlaybackInfo unavailable');
        }
        return apiClient.getPlaybackInfo(itemId);
    }

    async function getMainEpInfo(itemId) {
        const apiClient = pageWindow.ApiClient;
        const userId = apiClient?._serverInfo?.UserId;
        if (!apiClient || typeof apiClient.getItem !== 'function' || !userId) {
            throw new Error('ApiClient.getItem unavailable');
        }
        return apiClient.getItem(userId, itemId);
    }

    async function buildClickPlayData(itemId) {
        const [playbackData, mainEpInfo] = await Promise.all([
            getItemPlaybackInfo(itemId),
            getMainEpInfo(itemId),
        ]);
        if (!playbackData || !Array.isArray(playbackData.MediaSources) || playbackData.MediaSources.length === 0) {
            throw new Error('missing playback media sources');
        }
        const playbackURL = buildPlaybackURL(itemId, playbackData, mainEpInfo);
        return {
            playbackURL,
            playbackData,
            mainEpInfo,
            requestHeaders: {
                Referer: pageWindow.location.href,
            },
        };
    }

    function playNotify(title = '已调用 IINA', subtitle = '已转交本地播放器处理') {
        const doc = pageWindow.document;
        if (!doc || !doc.body) {
            return;
        }
        const notification = doc.createElement('div');
        notification.textContent = `${title} · ${subtitle}`;
        notification.style.cssText = [
            'position:fixed',
            'bottom:30px',
            'right:30px',
            'z-index:2147483647',
            'padding:14px 18px',
            'border-radius:12px',
            'background:linear-gradient(135deg,#0296be 0%,#008a51 100%)',
            'color:#fff',
            'font-size:14px',
            'box-shadow:0 10px 30px rgba(0,0,0,0.3)',
        ].join(';');
        doc.body.appendChild(notification);
        pageWindow.setTimeout(() => {
            notification.remove();
        }, 2500);
    }

    function isDuplicatePlay(playKey) {
        return state.lastPlayKey === playKey && now() - state.lastPlayAt < config.dedupeWindowMs;
    }

    function notify(message) {
        if (now() - state.lastNoticeAt < 2000) {
            return;
        }
        state.lastNoticeAt = now();
        console.warn('[emby-iina-only]', message);
        const doc = pageWindow.document;
        if (!doc || !doc.body) {
            return;
        }
        const toast = doc.createElement('div');
        toast.textContent = message;
        toast.style.cssText = [
            'position:fixed',
            'top:16px',
            'right:16px',
            'z-index:2147483647',
            'max-width:360px',
            'padding:10px 14px',
            'border-radius:8px',
            'background:rgba(35,35,35,0.92)',
            'color:#fff',
            'font-size:14px',
            'line-height:1.4',
            'box-shadow:0 6px 24px rgba(0,0,0,0.2)',
        ].join(';');
        doc.body.appendChild(toast);
        pageWindow.setTimeout(() => {
            toast.remove();
        }, 3500);
    }

    function removeErrorWindows() {
        const doc = pageWindow.document;
        if (!doc) {
            return false;
        }
        let changed = false;
        const okButtons = doc.querySelectorAll('button[data-id="ok"]');
        okButtons.forEach((button) => {
            if (!button || !button.textContent || !button.offsetParent) {
                return;
            }
            button.click();
            changed = true;
        });
        const spinner = doc.querySelector('div.docspinner');
        if (spinner) {
            spinner.remove();
            changed = true;
        }
        const playbackRoute = pageWindow.location.href;
        if (shouldSuppressPlaybackRoute(playbackRoute)) {
            pageWindow.history.replaceState(pageWindow.history.state, '', state.lastNonPlaybackURL);
            changed = true;
        }
        return changed;
    }

    async function removeErrorWindowsMultiTimes() {
        for (let index = 0; index < 15; index += 1) {
            await new Promise((resolve) => pageWindow.setTimeout(resolve, 200));
            if (removeErrorWindows()) {
                break;
            }
        }
    }

    async function postJSON(path, data) {
        const response = await originFetch(trimSlash(config.localServer) + path, {
            method: 'POST',
            mode: 'cors',
            headers: {
                'Content-Type': 'text/plain'
            },
            body: JSON.stringify(data),
        });
        return {
            status: response.status,
            responseText: await response.text(),
        };
    }

    function buildPayload(playbackURL, playbackData, requestHeaders, mainEpInfo) {
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
            playbackUrl: playbackURL,
            playbackData: {
                PlaySessionId: playbackData?.PlaySessionId || '',
                MediaSources: Array.isArray(playbackData?.MediaSources) ? playbackData.MediaSources : [],
            },
            request: {
                headers: requestHeaders,
            },
            extraData: {
                mainEpInfo: mainEpInfo || {},
            },
            mountDiskEnable: 'false',
            options,
        };
    }

    async function handleLocalPlay(playbackURL, playbackData, requestHeaders, mainEpInfo) {
        const payload = buildPayload(playbackURL, playbackData, requestHeaders, mainEpInfo);
        log('forward playback payload', payload);
        const response = await postJSON('/v1/emby/play', payload);
        if (response.status >= 200 && response.status < 300) {
            const playKey = buildPlayKey(playbackURL, playbackData);
            state.lastPlayKey = playKey;
            state.lastPlayAt = now();
            state.localPlaybackActive = true;
            log('forward playback success', playKey, response.responseText);
            return true;
        }
        throw new Error(`local play status=${response.status} body=${response.responseText || ''}`);
    }

    async function handleClickPlay(context) {
        if (state.clickPlayInFlight) {
            return;
        }
        state.clickPlayInFlight = true;
        state.suppressPlaybackErrorUntil = now() + 4000;
        state.suppressPlaybackRouteUntil = now() + 4000;
        try {
            const itemId = context.item?.Id || context.itemId;
            if (!itemId) {
                throw new Error('item id missing');
            }
            const playData = await buildClickPlayData(itemId);
            const playKey = buildPlayKey(playData.playbackURL, playData.playbackData);
            if (isDuplicatePlay(playKey) && state.localPlaybackActive) {
                log('duplicate click play ignored', playKey);
                return;
            }
            const forwarded = await handleLocalPlay(playData.playbackURL, playData.playbackData, playData.requestHeaders, playData.mainEpInfo);
            if (forwarded) {
                playNotify();
                void removeErrorWindowsMultiTimes();
                return;
            }
            throw new Error('local play not forwarded');
        } catch (error) {
            log('click play failed', error);
            markPassThroughClick();
            notify(`点击接管失败：${error?.message || error}`);
            context.playButton.click();
        } finally {
            state.clickPlayInFlight = false;
        }
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

    pageWindow.addEventListener('unhandledrejection', function (event) {
        if (!shouldSuppressPlaybackError(event.reason)) {
            return;
        }
        event.preventDefault();
        void removeErrorWindowsMultiTimes();
    }, true);

    pageWindow.addEventListener('popstate', function () {
        if (shouldSuppressPlaybackRoute(pageWindow.location.href)) {
            pageWindow.history.replaceState(pageWindow.history.state, '', state.lastNonPlaybackURL);
            void removeErrorWindowsMultiTimes();
        } else {
            updateLastNonPlaybackURL(pageWindow.location.href);
        }
    }, true);

    const originalPushState = pageWindow.history.pushState.bind(pageWindow.history);
    pageWindow.history.pushState = function (stateValue, title, urlValue) {
        if (typeof urlValue === 'string' && shouldSuppressPlaybackRoute(urlValue)) {
            return originalPushState(stateValue, title, state.lastNonPlaybackURL);
        }
        if (typeof urlValue === 'string') {
            updateLastNonPlaybackURL(urlValue);
        }
        return originalPushState(stateValue, title, urlValue);
    };

    const originalReplaceState = pageWindow.history.replaceState.bind(pageWindow.history);
    pageWindow.history.replaceState = function (stateValue, title, urlValue) {
        if (typeof urlValue === 'string' && shouldSuppressPlaybackRoute(urlValue)) {
            return originalReplaceState(stateValue, title, state.lastNonPlaybackURL);
        }
        if (typeof urlValue === 'string') {
            updateLastNonPlaybackURL(urlValue);
        }
        return originalReplaceState(stateValue, title, urlValue);
    };

    pageWindow.document.addEventListener('click', function (event) {
        const context = findPlayContext(event.target);
        if (!context || state.passThroughClickOnce || state.clickPlayInFlight) {
            return;
        }
        log('take over play click', context.itemId || context.item?.Id || '', context.playButton?.outerHTML || '');
        event.preventDefault();
        event.stopImmediatePropagation();
        void handleClickPlay(context);
    }, true);

    pageWindow.fetch = async function (input, init) {
        const url = parseURL(input);
        if (!url) {
            return originFetch(input, init);
        }

        updateLastNonPlaybackURL(pageWindow.location.href);

        if (isStopped(url) && state.localPlaybackActive) {
            const stopped = await handleLocalStop();
            if (stopped) {
                return buildJSONResponse({});
            }
            return originFetch(input, init);
        }

        return originFetch(input, init);
    };
})();
