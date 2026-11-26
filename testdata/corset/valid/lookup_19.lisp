(defcolumns (X1 :i16) (X2 :i16) (Y1 :i16) (Y2 :i16) (Z1 :i16) (Z2 :i16) (one :i1 :constant 1))
;; fragmented lookup with two two column targets
(deflookup test ((one X1 X2)) ((one Y1 Y2) (one Z1 Z2)))
