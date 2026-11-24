;;error:3:43-47:conflicting context
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(deflookup test ((one X Y)) ((m1.one m1.A m2.B)))

(module m1)
(defcolumns (A :i16) (one :i1 :constant 1))

(module m2)
(defcolumns (B :i16))
