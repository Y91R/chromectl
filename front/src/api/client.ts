import createClient from 'openapi-fetch'
import type { paths } from '../gen/api'

export const api = createClient<paths>({ baseUrl: '/api/v1' })
