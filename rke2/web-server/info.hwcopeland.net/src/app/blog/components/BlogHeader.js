import React, {useContext} from 'react'
import Link from 'next/link'

const Categories = [
{name:'Web Development', slug:'web-dev'},
{name:'Web Design', slug:'web-design'}
]


const BlogHeader = () => {
  return (
    <div className='container mx-auto px-10 mb-8'>
        <div className='border-b w-full inline-block border-blue-400 py-8'>
            <div className='md:float-left block mt-2'>
                <Link href='/Blog'>
                    <span className='cursor-pointer font-bold text-3xl text-white'>
                        Blogs and Articles
                    </span>
                </Link>
            </div>
            <div className='hidden md:float-left md:contents'>
                {Categories.map((category) => (
                    <Link href={`/category/${category.slug}`} key={category.slug}>
                        <span className='md:float-right mt-2 align-middle text-white ml-4 font-semibold cursor-pointer'>
                            {category.name}
                        </span>
                    </Link>
                ))}
            </div>
        </div>
    </div>
  )
}
export default BlogHeader
