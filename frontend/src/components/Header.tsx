import { Link } from 'react-router-dom'

export default function Header() {
  return (
    <header className="bg-gray-900 border-b border-gray-800 px-6 py-4">
      <Link to="/" className="inline-block no-underline">
        <h1 className="text-xl font-bold text-white m-0">GitCourse</h1>
        <p className="text-sm text-gray-400 mt-0.5">GitHub Actions Dashboard</p>
      </Link>
    </header>
  )
}
