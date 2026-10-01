type Point = {
  x: number
  y: number
}

type SavedLayout = Record<string, Point>
type StickerSoundName = 'pickup' | 'drop' | 'move' | 'reset' | 'cat'

type DragSession = {
  pointerId: number
  pointerType: string
  reduceMotion: boolean
  startClientX: number
  startClientY: number
  originX: number
  originY: number
  lastClientX: number
  lastClientY: number
  lastTime: number
  velocityX: number
  velocityY: number
  minX: number
  maxX: number
  minY: number
  maxY: number
  moved: boolean
}

type StickerState = {
  id: string
  element: HTMLButtonElement
  x: number
  y: number
  drag: DragSession | null
  frame: number | null
  pending: Point | null
  animation: Animation | null
  settleTimer: number | null
  dropSoundTimer: number | null
  suppressClick: boolean
}

const LAYOUT_STORAGE_KEY = 'aowugong:sticker-layout:go-sidebar:v3'
const SOUND_STORAGE_KEY = 'aowugong:sticker-sound:v2'
const SOUND_ASSETS: Record<StickerSoundName, { src: string; volume: number }> = {
  pickup: { src: '/blog-static/sounds/snowman-pickup.wav', volume: 0.68 },
  drop: { src: '/blog-static/sounds/snowman-drop.wav', volume: 0.78 },
  move: { src: '/blog-static/sounds/snowman-move.wav', volume: 0.42 },
  reset: { src: '/blog-static/sounds/snowman-reset.wav', volume: 0.62 },
  cat: { src: '/blog-static/sounds/cat.mp3', volume: 0.15 },
}
const INERTIA_PROJECTION_MS = 130
const MAX_INERTIA_DISTANCE = 84
const MAX_MOUSE_INERTIA_DISTANCE = 48
const MIN_INERTIA_SPEED = 0.12
const MAX_INERTIA_SPEED = 0.82
const RELEASE_VELOCITY_WINDOW_MS = 90
const MIN_INERTIA_DURATION = 150
const MAX_INERTIA_DURATION = 320
const SETTLE_DURATION = 360
const DROP_SOUND_DELAY = 166
const SNAP_BACK_DURATION = 330
const RUBBER_BAND_GIVE = 44
const MAX_DRAG_TILT = 7.5
const DRAG_TILT_FACTOR = 12
const RESET_DURATION = 470

const clamp = (value: number, minimum: number, maximum: number) =>
  Math.min(Math.max(value, minimum), maximum)

const rubberBand = (value: number, minimum: number, maximum: number) => {
  if (value < minimum) {
    const excess = minimum - value
    return minimum - RUBBER_BAND_GIVE * (1 - 1 / (1 + excess / RUBBER_BAND_GIVE))
  }
  if (value > maximum) {
    const excess = value - maximum
    return maximum + RUBBER_BAND_GIVE * (1 - 1 / (1 + excess / RUBBER_BAND_GIVE))
  }
  return value
}

const readLayout = (): SavedLayout => {
  try {
    return JSON.parse(
      window.localStorage.getItem(LAYOUT_STORAGE_KEY) ?? '{}'
    ) as SavedLayout
  } catch {
    return {}
  }
}

const writeLayout = (layout: SavedLayout) => {
  try {
    window.localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(layout))
  } catch {
    // 浏览器禁用存储时仍保留拖拽能力。
  }
}

const transformOf = ({ x, y }: Point) =>
  `translate3d(${x.toFixed(2)}px, ${y.toFixed(2)}px, 0)`

export const setupStickerWall = (wall: HTMLElement) => {
  const controller = new AbortController()
  function listen<K extends keyof HTMLElementEventMap>(element: HTMLElement, type: K, handler: (event: HTMLElementEventMap[K]) => void) {
    element.addEventListener(type, handler, { signal: controller.signal })
  }
  if (wall.dataset.initialized === 'true') return
  wall.dataset.initialized = 'true'

  const stage = wall.querySelector<HTMLElement>('[data-sticker-stage]')
  const resetButton = wall.querySelector<HTMLButtonElement>(
    '[data-sticker-reset]'
  )
  const soundButton = wall.querySelector<HTMLButtonElement>(
    '[data-sticker-sound]'
  )
  if (!stage || !resetButton || !soundButton) return

  let savedLayout = readLayout()
  let topLayer = 40
  let audioContext: AudioContext | null = null
  let soundEnabled = window.localStorage.getItem(SOUND_STORAGE_KEY) !== 'off'
  const soundCursor: Record<StickerSoundName, number> = {
    pickup: 0,
    drop: 0,
    move: 0,
    reset: 0,
    cat: 0,
  }
  const soundPools = Object.fromEntries(
    Object.entries(SOUND_ASSETS).map(([name, asset]) => [
      name,
      Array.from({ length: 3 }, () => {
        const audio = new Audio(asset.src)
        audio.preload = 'auto'
        audio.volume = asset.volume
        audio.load()
        return audio
      }),
    ])
  ) as Record<StickerSoundName, HTMLAudioElement[]>
  const reduceMotion = window.matchMedia(
    '(prefers-reduced-motion: reduce)'
  )

  const states: StickerState[] = Array.from(
    wall.querySelectorAll<HTMLButtonElement>('[data-sticker]')
  ).map((element) => {
    const id = element.dataset.sticker ?? ''
    const saved = savedLayout[id] ?? { x: 0, y: 0 }
    const state: StickerState = {
      id,
      element,
      x: saved.x,
      y: saved.y,
      drag: null,
      frame: null,
      pending: null,
      animation: null,
      settleTimer: null,
      dropSoundTimer: null,
      suppressClick: false,
    }
    element.style.transform = transformOf(saved)
    return state
  })

  const updateSoundButton = () => {
    soundButton.setAttribute('aria-pressed', String(soundEnabled))
    soundButton.dataset.enabled = String(soundEnabled)
  }

  const getAudioContext = () => {
    if (audioContext) return audioContext
    const AudioContextClass =
      window.AudioContext ??
      (window as typeof window & { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext
    if (!AudioContextClass) return null
    audioContext = new AudioContextClass()
    return audioContext
  }

  const playSnowFallback = (name: StickerSoundName) => {
    if (!soundEnabled) return
    if (name === 'cat') return
    const context = getAudioContext()
    if (!context) return
    void context.resume()
    const durations: Record<Exclude<StickerSoundName, 'cat'>, number> = {
      pickup: 0.2,
      drop: 0.34,
      move: 0.12,
      reset: 0.46,
    }
    const duration = durations[name]
    const buffer = context.createBuffer(
      1,
      Math.ceil(context.sampleRate * duration),
      context.sampleRate
    )
    const samples = buffer.getChannelData(0)
    let seed = 0x6d2b79f5
    let softNoise = 0
    for (let index = 0; index < samples.length; index += 1) {
      seed ^= seed << 13
      seed ^= seed >>> 17
      seed ^= seed << 5
      const whiteNoise = ((seed >>> 0) / 4_294_967_295) * 2 - 1
      softNoise = softNoise * 0.9 + whiteNoise * 0.1
      const time = index / context.sampleRate
      const envelope =
        (1 - Math.exp(-time / 0.012)) * Math.exp(-time / (duration * 0.42))
      samples[index] = (softNoise * 0.9 + whiteNoise * 0.08) * envelope
    }

    const source = context.createBufferSource()
    source.buffer = buffer
    const filter = context.createBiquadFilter()
    filter.type = 'lowpass'
    filter.frequency.value = 1800
    const gain = context.createGain()
    gain.gain.value = name === 'drop' ? 0.42 : 0.28
    source.connect(filter)
    filter.connect(gain)
    gain.connect(context.destination)
    source.start()
  }

  const playSound = (name: StickerSoundName) => {
    if (!soundEnabled) return

    const context = getAudioContext()
    if (context?.state === 'suspended') void context.resume()

    const pool = soundPools[name]
    const player = pool[soundCursor[name] % pool.length]
    soundCursor[name] += 1
    player.pause()
    player.currentTime = 0
    player.volume = SOUND_ASSETS[name].volume
    void player.play().catch(() => playSnowFallback(name))
  }

  const cancelDropSound = (state: StickerState) => {
    if (state.dropSoundTimer === null) return
    window.clearTimeout(state.dropSoundTimer)
    state.dropSoundTimer = null
  }

  const scheduleDropSound = (state: StickerState, immediate: boolean) => {
    cancelDropSound(state)
    state.dropSoundTimer = window.setTimeout(() => {
      state.dropSoundTimer = null
      playSound('drop')
    }, immediate ? 0 : DROP_SOUND_DELAY)
  }

  const persistState = (state: StickerState) => {
    savedLayout[state.id] = { x: state.x, y: state.y }
    writeLayout(savedLayout)
  }

  const commitPosition = (state: StickerState, point: Point) => {
    state.x = point.x
    state.y = point.y
    state.element.style.transform = transformOf(point)
  }

  const schedulePosition = (state: StickerState, point: Point) => {
    state.pending = point
    if (state.frame !== null) return
    state.frame = window.requestAnimationFrame(() => {
      state.frame = null
      if (!state.pending) return
      commitPosition(state, state.pending)
      state.pending = null
    })
  }

  const flushPosition = (state: StickerState) => {
    if (state.frame !== null) {
      window.cancelAnimationFrame(state.frame)
      state.frame = null
    }
    if (state.pending) {
      commitPosition(state, state.pending)
      state.pending = null
    }
  }

  const boundsFor = (state: StickerState) => {
    const stageRect = stage.getBoundingClientRect()
    const stickerRect = state.element.getBoundingClientRect()
    const shadowPad = Number(state.element.dataset.shadowPad ?? 0)
    const inset = Math.min(shadowPad, Math.max(document.documentElement.clientWidth / 2 - 1, 0))
    const pageRect = document.body.getBoundingClientRect()
    return {
      minX: state.x + inset - stickerRect.left,
      maxX: state.x + document.documentElement.clientWidth - inset - stickerRect.right,
      minY:
        state.y + Math.min(pageRect.top, stageRect.top) - stickerRect.top,
      maxY:
        state.y + Math.max(pageRect.bottom, stageRect.bottom) - stickerRect.bottom,
    }
  }

  const cancelAnimation = (state: StickerState) => {
    const animation = state.animation
    if (animation) {
      const renderedTransform = window.getComputedStyle(
        state.element
      ).transform
      let renderedPosition = { x: state.x, y: state.y }
      if (renderedTransform !== 'none') {
        try {
          const matrix = new DOMMatrixReadOnly(renderedTransform)
          renderedPosition = { x: matrix.m41, y: matrix.m42 }
        } catch {
          // 无法解析时保留上一次已提交的位置。
        }
      }
      state.animation = null
      animation.cancel()
      commitPosition(state, renderedPosition)
    }
    if (state.settleTimer !== null) {
      window.clearTimeout(state.settleTimer)
      state.settleTimer = null
    }
    state.element.dataset.settling = 'false'
  }

  const settleFor = (state: StickerState, duration = SETTLE_DURATION) => {
    if (state.settleTimer !== null) window.clearTimeout(state.settleTimer)
    state.element.dataset.settling = 'true'
    state.settleTimer = window.setTimeout(() => {
      state.settleTimer = null
      state.element.dataset.settling = 'false'
    }, duration)
  }

  const animateTo = (
    state: StickerState,
    target: Point,
    options: {
      duration?: number
      delay?: number
      persist?: boolean
      easing?: string
      settleDuration?: number
    } = {}
  ) => {
    flushPosition(state)
    cancelAnimation(state)
    const duration = reduceMotion.matches ? 0 : (options.duration ?? 260)
    const delay = reduceMotion.matches ? 0 : (options.delay ?? 0)

    if (duration === 0) {
      commitPosition(state, target)
      if (options.persist !== false) persistState(state)
      return Promise.resolve()
    }

    if (options.settleDuration) settleFor(state, options.settleDuration)
    const animation = state.element.animate(
      [
        { transform: transformOf(state) },
        { transform: transformOf(target) },
      ],
      {
        duration,
        delay,
        easing: options.easing ?? 'cubic-bezier(0.23, 1, 0.32, 1)',
        fill: 'both',
      }
    )
    state.animation = animation

    return animation.finished
      .catch(() => undefined)
      .then(() => {
        if (state.animation !== animation) return
        state.animation = null
        // 已完成且带 fill 的动画仍会覆盖后续写入的 inline transform。
        // 先移除动画效果，再把终点固化，保证下一次拖拽立即生效。
        animation.cancel()
        commitPosition(state, target)
        if (options.persist !== false) persistState(state)
      })
  }

  const bringToFront = (state: StickerState) => {
    topLayer += 1
    state.element.style.zIndex = String(topLayer)
  }

  const abortDrags: Array<() => void> = []

  for (const state of states) {
    const { element } = state

    listen(element, 'pointerdown', (event) => {
      if (
        !event.isPrimary ||
        state.drag !== null ||
        (event.pointerType === 'mouse' && event.button !== 0)
      ) {
        return
      }

      flushPosition(state)
      cancelAnimation(state)
      cancelDropSound(state)
      const bounds = boundsFor(state)
      const rect = element.getBoundingClientRect()
      state.drag = {
        pointerId: event.pointerId,
        pointerType: event.pointerType,
        reduceMotion: reduceMotion.matches,
        startClientX: event.clientX,
        startClientY: event.clientY,
        originX: state.x,
        originY: state.y,
        lastClientX: event.clientX,
        lastClientY: event.clientY,
        lastTime: event.timeStamp,
        velocityX: 0,
        velocityY: 0,
        moved: false,
        ...bounds,
      }

      element.style.setProperty(
        '--grab-x',
        `${clamp(((event.clientX - rect.left) / rect.width) * 100, 18, 82)}%`
      )
      element.style.setProperty(
        '--grab-y',
        `${clamp(((event.clientY - rect.top) / rect.height) * 100, 20, 78)}%`
      )
      element.dataset.dragging = 'true'
      element.setPointerCapture(event.pointerId)
      bringToFront(state)
      playSound(state.id === 'cat' ? 'cat' : 'pickup')
    })

    function handlePointerMove(event: PointerEvent) {
      const drag = state.drag
      if (!drag || drag.pointerId !== event.pointerId) return
      event.preventDefault()

      const deltaTime = Math.max(event.timeStamp - drag.lastTime, 16)
      const instantVelocityX = (event.clientX - drag.lastClientX) / deltaTime
      const instantVelocityY = (event.clientY - drag.lastClientY) / deltaTime
      drag.velocityX = drag.velocityX * 0.62 + instantVelocityX * 0.38
      drag.velocityY = drag.velocityY * 0.62 + instantVelocityY * 0.38
      drag.lastClientX = event.clientX
      drag.lastClientY = event.clientY
      drag.lastTime = event.timeStamp

      const deltaX = event.clientX - drag.startClientX
      const deltaY = event.clientY - drag.startClientY
      if (Math.hypot(deltaX, deltaY) > 3) drag.moved = true
      schedulePosition(state, {
        x: rubberBand(drag.originX + deltaX, drag.minX, drag.maxX),
        y: rubberBand(drag.originY + deltaY, drag.minY, drag.maxY),
      })
      element.style.setProperty(
        '--drag-tilt',
        `${clamp(
          drag.reduceMotion ? 0 : drag.velocityX * DRAG_TILT_FACTOR,
          -MAX_DRAG_TILT,
          MAX_DRAG_TILT
        ).toFixed(2)}deg`
      )
    }

    function finishDrag(event: PointerEvent) {
      const drag = state.drag
      if (!drag || drag.pointerId !== event.pointerId) return
      flushPosition(state)
      state.drag = null
      state.suppressClick = drag.moved
      element.dataset.dragging = 'false'
      element.style.setProperty('--drag-tilt', '0deg')
      const home = {
        x: clamp(state.x, drag.minX, drag.maxX),
        y: clamp(state.y, drag.minY, drag.maxY),
      }
      const overshoot = Math.hypot(home.x - state.x, home.y - state.y)

      if (drag.reduceMotion) {
        commitPosition(state, home)
        persistState(state)
      } else if (overshoot > 0.5) {
        void animateTo(state, home, {
          duration: SNAP_BACK_DURATION,
          easing: 'cubic-bezier(0.2, 0.9, 0.3, 1.08)',
          settleDuration: Math.max(
            SETTLE_DURATION,
            SNAP_BACK_DURATION + 60
          ),
        })
      } else {
        const releaseDelay = Math.max(event.timeStamp - drag.lastTime, 0)
        const freshness =
          releaseDelay >= RELEASE_VELOCITY_WINDOW_MS
            ? 0
            : 1 - releaseDelay / RELEASE_VELOCITY_WINDOW_MS
        const velocityX = clamp(
          drag.velocityX * freshness ** 2,
          -MAX_INERTIA_SPEED,
          MAX_INERTIA_SPEED
        )
        const velocityY = clamp(
          drag.velocityY * freshness ** 2,
          -MAX_INERTIA_SPEED,
          MAX_INERTIA_SPEED
        )
        const speed = Math.hypot(velocityX, velocityY)

        if (speed < MIN_INERTIA_SPEED) {
          persistState(state)
          settleFor(state)
        } else {
          const maxDistance =
            drag.pointerType === 'mouse'
              ? MAX_MOUSE_INERTIA_DISTANCE
              : MAX_INERTIA_DISTANCE
          const target = {
            x: clamp(
              state.x +
                clamp(
                  velocityX * INERTIA_PROJECTION_MS,
                  -maxDistance,
                  maxDistance
                ),
              drag.minX,
              drag.maxX
            ),
            y: clamp(
              state.y +
                clamp(
                  velocityY * INERTIA_PROJECTION_MS,
                  -maxDistance,
                  maxDistance
                ),
              drag.minY,
              drag.maxY
            ),
          }
          const distance = Math.hypot(target.x - state.x, target.y - state.y)

          if (distance < 4) {
            persistState(state)
            settleFor(state)
          } else {
            const duration = clamp(
              MIN_INERTIA_DURATION + distance * 1.15,
              MIN_INERTIA_DURATION,
              MAX_INERTIA_DURATION
            )
            void animateTo(state, target, {
              duration,
              settleDuration: Math.max(SETTLE_DURATION, duration + 80),
            })
          }
        }
      }
      if (state.id !== 'cat') {
        scheduleDropSound(state, drag.reduceMotion)
      }
      if (element.hasPointerCapture(event.pointerId)) {
        element.releasePointerCapture(event.pointerId)
      }
    }

    function abortDrag() {
      const drag = state.drag
      cancelDropSound(state)
      if (!drag) return

      flushPosition(state)
      state.drag = null
      state.suppressClick = false
      element.dataset.dragging = 'false'
      element.style.setProperty('--drag-tilt', '0deg')
      if (element.hasPointerCapture(drag.pointerId)) {
        element.releasePointerCapture(drag.pointerId)
      }
      persistState(state)
    }

    abortDrags.push(abortDrag)

    listen(element, 'pointermove', handlePointerMove)
    listen(element, 'pointerup', finishDrag)
    listen(element, 'pointercancel', finishDrag)
    listen(element, 'lostpointercapture', (event) => {
      if (state.drag?.pointerId === event.pointerId) finishDrag(event)
    })

    listen(element, 'click', (event) => {
      if (!state.suppressClick) return
      event.preventDefault()
      state.suppressClick = false
    })

    listen(element, 'keydown', (event) => {
      const direction: Record<string, Point> = {
        ArrowLeft: { x: -1, y: 0 },
        ArrowRight: { x: 1, y: 0 },
        ArrowUp: { x: 0, y: -1 },
        ArrowDown: { x: 0, y: 1 },
      }
      const vector = direction[event.key]
      if (!vector) return
      event.preventDefault()
      cancelAnimation(state)
      const amount = event.shiftKey ? 36 : 12
      const bounds = boundsFor(state)
      const target = {
        x: clamp(state.x + vector.x * amount, bounds.minX, bounds.maxX),
        y: clamp(state.y + vector.y * amount, bounds.minY, bounds.maxY),
      }
      bringToFront(state)
      commitPosition(state, target)
      persistState(state)
    })
  }

  listen(resetButton, 'click', async () => {
    abortDrags.forEach((abortDrag) => abortDrag())
    savedLayout = {}
    writeLayout(savedLayout)
    resetButton.disabled = true
    playSound('reset')
    await Promise.all(
      states.map((state, index) =>
        animateTo(state, { x: 0, y: 0 }, {
          delay: index * 45,
          duration: RESET_DURATION,
          easing: 'cubic-bezier(0.3, 1.06, 0.3, 1)',
          persist: false,
        })
      )
    )
    resetButton.disabled = false
    resetButton.focus()
  })

  listen(soundButton, 'click', () => {
    soundEnabled = !soundEnabled
    window.localStorage.setItem(
      SOUND_STORAGE_KEY,
      soundEnabled ? 'on' : 'off'
    )
    updateSoundButton()
  })

  const images = Array.from(
    wall.querySelectorAll<HTMLImageElement>('[data-sticker-image]')
  )
  void Promise.allSettled(
    images.map((image) =>
      typeof image.decode === 'function' ? image.decode() : Promise.resolve()
    )
  ).then(() => {
    if (!controller.signal.aborted) wall.dataset.ready = 'true'
  })

  updateSoundButton()
  return () => {
    controller.abort()
    states.forEach((state) => {
      if (state.frame !== null) cancelAnimationFrame(state.frame)
      if (state.settleTimer !== null) clearTimeout(state.settleTimer)
      if (state.dropSoundTimer !== null) clearTimeout(state.dropSoundTimer)
      state.animation?.cancel()
    })
    Object.values(soundPools).flat().forEach((audio) => { audio.pause(); audio.removeAttribute('src'); audio.load() })
    if (audioContext) void audioContext.close()
    delete wall.dataset.initialized
  }

}
