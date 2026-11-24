;;error:8:23-27:conflicting context
(module m1)
(defcolumns (X :i16) (Y :i16) (Z :i16) (one :i1 :constant 1))

(module m2)
(defcolumns (A :i16) (one :i1 :constant 1))

(deflookup l1 ((one A m1.Z)) ((m1.one m1.X m1.Y)))
