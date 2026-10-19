import { t } from '../i18n.js'
// Preserve the selected files. Pixel processing belongs to the shared backend
// converter so preview, printing and later reprints use the same policy.
export function isPhotoFile(file) {
  return !!file && (file.type?.startsWith('image/') || /\.(jpe?g|png|gif|heic|heif)$/i.test(file.name || ''))
}

export function appendPhotoUploads(form, files, { rotations = [], perPage = 1, name = '' } = {}) {
  if (!files.length) throw new Error(t('ui.select_photos_first'))
  for (const file of files) form.append(files.length === 1 ? 'file' : 'files', file, file.name)
  form.append('rotations', files.map((_, i) => rotations?.[i] || 0).join(','))
  form.append('per_page', String(perPage || 1))
  if (name) form.append('name', name)
}
