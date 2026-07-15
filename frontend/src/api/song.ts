import type { Song } from '../types'

/**
 * 获取歌曲的播放 URL
 */
export async function fetchSongUrl(song: Song): Promise<string> {
  if (song.url) return song.url
  const resp = await fetch(
    `/api/song/url?source=${encodeURIComponent(song.source || 'kuwo')}&musicId=${encodeURIComponent(song.id)}`
  )
  const data = await resp.json()
  return data.url || ''
}

/**
 * 搜索歌曲
 */
export async function searchSongsApi(query: string, sources: string): Promise<Song[]> {
  const resp = await fetch(`/api/search?q=${encodeURIComponent(query)}&sources=${sources}`)
  const data = await resp.json()
  return (data.results || []).filter(
    (v: Song) => v.source === 'netease' || v.source === 'qq' || v.source === 'kuwo' || v.source === 'kugou'
  )
}

/**
 * 搜索歌单
 */
export async function searchPlaylistsApi(query: string): Promise<any[]> {
  const resp = await fetch(`/api/search?q=${encodeURIComponent(query)}&type=playlist&sources=netease`)
  const data = await resp.json()
  return data.results || []
}

/**
 * 获取歌单详情
 */
export async function getPlaylistDetailApi(id: string, source: string): Promise<Song[]> {
  const resp = await fetch(`/api/playlist/detail?id=${id}&source=${source}`)
  const data = await resp.json()
  return data.results || []
}

/**
 * 获取歌词
 */
export async function getLyricApi(songId: string, source: string): Promise<any> {
  const resp = await fetch(`/api/lyric?id=${encodeURIComponent(songId)}&source=${encodeURIComponent(source)}`)
  return resp.json()
}
