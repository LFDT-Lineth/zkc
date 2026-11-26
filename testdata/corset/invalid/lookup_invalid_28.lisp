;;error:8:21-25:conflicting context
(module m1)
(defcolumns (X :i16) (Y :i16) (Z :i16))

(module m2)
(defcolumns (A :i16))

(deflookup l1 ((_ A m1.Z)) ((_ m1.X m1.Y)))
